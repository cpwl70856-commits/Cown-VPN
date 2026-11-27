// Command client launches the mywgclient Fyne desktop WireGuard client.
//
// Creating a TUN device typically requires elevated privileges
// (root on Linux/macOS, Administrator on Windows with Wintun installed).
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/tunnel"
	"github.com/user/mywgclient/internal/ui"
)

func main() {
	cfgPath := flag.String("config", "", "optional WireGuard .conf for headless connect (skips GUI)")
	timeout := flag.Duration("timeout", 20*time.Second, "connect timeout when using -config")
	routeMode := flag.String("route-mode", "split", "headless route mode: split|global")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if *cfgPath != "" {
		runHeadless(*cfgPath, *timeout, *routeMode)
		return
	}
	ui.Run()
}

func runHeadless(path string, timeout time.Duration, routeMode string) {
	cfg, err := config.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	t := tunnel.New()
	defer t.Disconnect()

	mode := tunnel.RouteModeSplit
	if routeMode == "global" {
		mode = tunnel.RouteModeGlobal
	}

	endpoint := ""
	if p := cfg.PrimaryPeer(); p != nil {
		endpoint = p.Endpoint
	}
	slog.Info("connecting", "endpoint", endpoint, "addr", cfg.Address, "route_mode", mode)
	if err := t.ConnectWithOptions(cfg, timeout, tunnel.ConnectOptions{RouteMode: mode}); err != nil {
		slog.Error("connect failed", "err", err)
		os.Exit(1)
	}
	slog.Info("connected", "iface", t.Name(), "addr", t.Address(), "routes", len(t.AppliedRoutes()))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		slog.Info("signal received, cleaning routes and exiting", "signal", sig.String())
	case <-t.Wait():
		slog.Warn("tunnel device closed unexpectedly")
	}
}
