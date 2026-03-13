# bento-aprs

A Bento input plugin for connecting to APRS-IS servers and streaming parsed APRS packets.

## About APRS

APRS (Automatic Packet Reporting System) is a digital communications system used by amateur radio operators to share real-time information. It can transmit various types of data including:

- **Weather data** - Temperature, pressure, wind speed, rainfall
- **Position tracking** - GPS coordinates of stations, vehicles, and objects
- **Messages** - Text messaging between stations
- **Telemetry** - Sensor data and status information
- **Objects** - Points of interest on maps

APRS-IS (APRS Internet Service) is the network of servers that relay APRS data over the internet, allowing global access to APRS information without requiring a radio receiver.

## Features

- Connects to any APRS-IS server
- Parses packets using [go-aprs-fap](https://github.com/hessu/go-aprs-fap)
- Outputs structured JSON messages
- Supports APRS-IS filters for targeted data
- Handles connection failures with automatic reconnection
- Metadata extraction for routing (source callsign, destination, packet type)

## Configuration

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `address` | string | `rotate.aprs2.net:14580` | APRS-IS server address |
| `callsign` | string | required | Your amateur radio callsign |
| `passcode` | string | `-1` | APRS-IS passcode (use `-1` for read-only) |
| `app_name` | string | `bento-aprs` | Application name sent during login |
| `app_version` | string | `1.0.0` | Application version sent during login |
| `filter` | string | `""` | Optional APRS-IS filter string |

### Output Format

Each received packet is output as a JSON message containing the parsed `fap.Packet` struct. Metadata fields are also set:

- `@aprs_src` - Source callsign
- `@aprs_dst` - Destination callsign
- `@aprs_type` - Packet type

If a packet fails to parse, the raw packet is output with `@aprs_error` metadata containing the error message.

## Example Configuration

```yaml
input:
  aprs_is:
    address: "rotate.aprs2.net:14580"
    callsign: "N0CALL"
    passcode: "-1"
    filter: "r/46.82/-71.25/200"

pipeline:
  processors:
    # Client-side filter: only pass weather packets
    - mapping: |if this.Wx == null || (this.Wx.Temp == null && this.Wx.Pressure == null) {
          root = deleted()
        } else {
          root.source = this.SrcCallsign
          root.latitude = this.Latitude
          root.longitude = this.Longitude
          root.pressure = this.Wx.Pressure
          root.temp = this.Wx.Temp
        }

output:
  switch:
    cases:
      # Route errors to stderr
      - check: '@aprs_error != null'
        output:
          stdout:
            codec: lines
      # Route valid data to stdout
      - output:
          stdout:
            codec: lines
```

## Filter Examples

APRS-IS filters allow you to limit the data received:

| Filter | Description |
|--------|-------------|
| `r/50/10/500` | Range: within 500km of lat 50, lon 10 |
| `t/w` | Type: weather packets only |
| `t/po` | Type: position and objects |
| `b/ABCDEF` | Budlist: packets from specific stations |
| `p/N0CALL` | Prefix: packets from callsign prefix |

### Important: Combining Range and Type Filters

APRS-IS servers do not properly AND `t/w` (weather) and `r/` (range) filters together. When both are specified, the range filter is often ignored.

**Workaround**: Use range filter (`r/`) on the server side and filter for weather packets client-side in your Bento pipeline (see example configuration above).

## Building

```bash
go build -o bento-aprs .
```

## Usage

```bash
./bento-aprs -c aprs.yaml
```

## License

MIT
