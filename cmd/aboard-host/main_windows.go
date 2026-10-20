//go:build windows && amd64

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"

	"github.com/abworks-dev/aboard-host/internal/host"
	"github.com/abworks-dev/aboard-host/internal/installation"
)

func execute(args []string, out, diagnostics io.Writer) int {
	if len(args) != 3 || (args[0] != "init" && args[0] != "serve") || args[1] != "--config" {
		fmt.Fprintln(diagnostics, "usage: aboard-host init|serve --config <trusted-config.json>")
		return 2
	}
	config, err := host.LoadConfig(args[2])
	if err != nil {
		fmt.Fprintln(diagnostics, "INVALID_CONFIGURATION")
		return 2
	}
	var mu sync.Mutex
	encoder := json.NewEncoder(out)
	log := func(e host.Event) { mu.Lock(); defer mu.Unlock(); _ = encoder.Encode(e) }
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if args[0] == "init" {
		err = host.Initialize(ctx, config, log)
	} else {
		err = host.Run(ctx, config, log)
	}
	if err != nil {
		fmt.Fprintln(diagnostics, "HOST_UNAVAILABLE")
		return 1
	}
	return 0
}
func main() {
	installation.InstallAcceptanceHTTPObservation()
	os.Exit(execute(os.Args[1:], os.Stdout, os.Stderr))
}
