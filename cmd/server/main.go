// Command server runs a minimal userspace WireGuard peer for testing mywgclient.
//
// Example:
//
//	go run ./cmd/keygen
//	# fill config/server.example.conf with matching keys
//	sudo go run ./cmd/server -config config/server.example.conf
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
)

func main() {
	cfgPath := flag.String("config", "config/server.example.conf", "WireGuard conf path")
	statusEvery := flag.Duration("status", 10*time.Second, "print RX/TX status interval (0 to disable)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.LoadFile(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg.ListenPort <= 0 {
		cfg.ListenPort = config.DefaultListenPort
	}

	slog.Info("mywgclient test server starting",
		"listen_port", cfg.ListenPort,
		"address", cfg.Address,
		"peers", len(cfg.Peers),
	)

	t, err := tunnel.StartServer(cfg)
	if err != nil {
		slog.Error("tunnel start failed", "err", err)
		os.Exit(1)
	}
	defer t.Disconnect()

	slog.Info("server ready", "iface", t.Name(), "addr", t.Address())
	slog.Info("enable IP forwarding / NAT on the host if you need internet access for clients")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var statusCh <-chan time.Time
	var ticker *time.Ticker
	if *statusEvery > 0 {
		ticker = time.NewTicker(*statusEvery)
		defer ticker.Stop()
		statusCh = ticker.C
	}

	for {
		select {
		case sig := <-sigCh:
			slog.Info("signal received, exiting", "signal", sig.String())
			return
		case <-t.Wait():
			slog.Warn("tunnel device closed unexpectedly")
			return
		case <-statusCh:
			st, err := t.Stats()
			if err != nil {
				continue
			}
			hs := "never"
			if st.LastHandshakeUnix > 0 {
				hs = time.Unix(st.LastHandshakeUnix, 0).Format(time.RFC3339)
			}
			slog.Info("status",
				"rx_bytes", st.RxBytes,
				"tx_bytes", st.TxBytes,
				"last_handshake", hs,
				"iface", t.Name(),
			)
		}
	}
}
