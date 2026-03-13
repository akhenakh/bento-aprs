package plugins

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	fap "github.com/hessu/go-aprs-fap"
	"github.com/warpstreamlabs/bento/public/service"
)

func aprsInputSpec() *service.ConfigSpec {
	return service.NewConfigSpec().
		Summary("Connects to an APRS-IS server and streams parsed APRS packets as JSON.").
		Fields(
			service.NewStringField("address").Default("rotate.aprs2.net:14580").Description("The APRS-IS server address to connect to."),
			service.NewStringField("callsign").Description("Your APRS callsign (e.g., 'N0CALL')."),
			service.NewStringField("passcode").Default("-1").Description("Your APRS-IS passcode. Use '-1' for read-only access."),
			service.NewStringField("app_name").Default("bento-aprs").Description("The application name sent during the APRS-IS login."),
			service.NewStringField("app_version").Default("1.0.0").Description("The application version sent during the APRS-IS login."),
			service.NewStringField("filter").Default("").Description("An optional APRS-IS filter string (e.g., 'r/50/10/500' for a range filter)."),
		)
}

func init() {
	err := service.RegisterInput("aprs_is", aprsInputSpec(),
		func(pConf *service.ParsedConfig, res *service.Resources) (service.Input, error) {
			return newAprsInput(pConf, res.Logger())
		})
	if err != nil {
		panic(err)
	}
}

type aprsInput struct {
	address    string
	callsign   string
	passcode   string
	appName    string
	appVersion string
	filter     string

	conn   *fap.Conn
	logger *service.Logger
}

func newAprsInput(conf *service.ParsedConfig, logger *service.Logger) (service.Input, error) {
	address, err := conf.FieldString("address")
	if err != nil {
		return nil, err
	}
	callsign, err := conf.FieldString("callsign")
	if err != nil {
		return nil, err
	}
	passcode, err := conf.FieldString("passcode")
	if err != nil {
		return nil, err
	}
	appName, err := conf.FieldString("app_name")
	if err != nil {
		return nil, err
	}
	appVersion, err := conf.FieldString("app_version")
	if err != nil {
		return nil, err
	}
	filter, err := conf.FieldString("filter")
	if err != nil {
		return nil, err
	}

	i := &aprsInput{
		address:    address,
		callsign:   callsign,
		passcode:   passcode,
		appName:    appName,
		appVersion: appVersion,
		filter:     filter,
		logger:     logger,
	}

	// AutoRetryNacks handles message acks/nacks cleanly for us.
	return service.AutoRetryNacks(i), nil
}

func (a *aprsInput) Connect(ctx context.Context) error {
	if a.conn != nil {
		return nil
	}

	a.logger.Infof("Connecting to APRS-IS at %s as %s", a.address, a.callsign)

	var conn *fap.Conn
	var err error

	if a.filter != "" {
		conn, err = fap.Dial(a.address, a.callsign, a.passcode, a.appName, a.appVersion, a.filter)
	} else {
		conn, err = fap.Dial(a.address, a.callsign, a.passcode, a.appName, a.appVersion)
	}

	if err != nil {
		return err
	}

	a.conn = conn
	return nil
}

func (a *aprsInput) Read(ctx context.Context) (*service.Message, service.AckFunc, error) {
	if a.conn == nil {
		return nil, nil, service.ErrNotConnected
	}

	for {
		// Check if the context was cancelled before attempting a read
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		// ReadPacket blocks up to the provided timeout duration
		raw, err := a.conn.ReadPacket(time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}

			errStr := strings.ToLower(err.Error())

			// Ignore standard read timeouts and try again
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
				continue
			}

			// For EOF or other connection-breaking errors, force a reconnect
			a.logger.Errorf("Connection to APRS-IS broken: %v", err)
			a.conn.Close()
			a.conn = nil
			return nil, nil, service.ErrNotConnected
		}

		// Silently skip completely empty lines
		if strings.TrimSpace(raw) == "" {
			continue
		}

		// Parse the packet
		parsed, err := fap.Parse(raw)
		if err != nil {
			// Some packets might fail to parse due to corruption or invalid formatting.
			// We yield them as raw strings and attach an `aprs_error` to the metadata,
			// allowing the Bento pipeline to inspect, log, or drop them.
			msg := service.NewMessage([]byte(raw))
			msg.MetaSet("aprs_error", err.Error())
			return msg, func(context.Context, error) error { return nil }, nil
		}

		// Marshal the parsed fap.Packet struct into a JSON byte array
		j, err := json.Marshal(parsed)
		if err != nil {
			a.logger.Errorf("Failed to marshal parsed packet to JSON: %v", err)
			continue
		}

		// Wrap the result into a Bento message
		msg := service.NewMessage(j)

		// Set useful routing metadata extracted from the packet
		msg.MetaSet("aprs_src", parsed.SrcCallsign)
		msg.MetaSet("aprs_dst", parsed.DstCallsign)
		msg.MetaSet("aprs_type", string(parsed.Type))

		// A dummy ack-func is returned since APRS-IS doesn't have a concept of packet acknowledgment
		return msg, func(context.Context, error) error { return nil }, nil
	}
}

func (a *aprsInput) Close(ctx context.Context) error {
	if a.conn != nil {
		a.logger.Info("Closing connection to APRS-IS")
		err := a.conn.Close()
		a.conn = nil
		return err
	}
	return nil
}
