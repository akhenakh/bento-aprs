package main

import (
	"context"

	// Import base bento components
	_ "github.com/warpstreamlabs/bento/public/components/all"

	"github.com/warpstreamlabs/bento/public/service"

	_ "github.com/akhenakh/bento-aprs/aprs"
)

func main() {
	service.RunCLI(context.Background())
}
