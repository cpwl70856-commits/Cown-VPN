package tunnel

import (
	"io"
	"os"
	"testing"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/crypto"

	"golang.zx2c4.com/wireguard/conn/bindtest"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/tuntest"
)

func TestUpFromDeviceRejectsNilTUN(t *testing.T) {
	err := New().UpFromDevice(sampleTunnelConfig(t), nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpFromDeviceInvalidConfigClosesTUN(t *testing.T) {
	dev := &recordingTUN{}
	err := New().UpFromDevice(&config.Config{}, dev, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !dev.closed {
		t.Fatal("invalid config must close the TUN")
	}
}

func TestUpFromDeviceChannelTUN(t *testing.T) {
	cfg := sampleTunnelConfig(t)
	ctun := tuntest.NewChannelTUN()
	binds := bindtest.NewChannelBinds()
	tun := New()
	if err := tun.UpFromDevice(cfg, ctun.TUN(), binds[0]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tun.Disconnect() })

	if tun.Status() != StatusConnected {
		t.Fatalf("status %s", tun.Status())
	}
	if tun.Address() != "10.8.0.2/24" {
		t.Fatalf("address %s", tun.Address())
	}
	if tun.Name() != "loopbackTun1" {
		t.Fatalf("name %s", tun.Name())
	}
	if mode := tun.RouteMode(); mode != RouteModeNone {
		t.Fatalf("route mode %s", mode)
	}
	if routes := tun.AppliedRoutes(); len(routes) != 0 {
		t.Fatalf("routes %v", routes)
	}
	if _, err := tun.Stats(); err != nil {
		t.Fatal(err)
	}

	again := tuntest.NewChannelTUN()
	err := tun.UpFromDevice(cfg, again.TUN(), binds[1])
	if err == nil {
		t.Fatal("second up must fail while connected")
	}

	if err := tun.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if tun.Status() != StatusDisconnected {
		t.Fatalf("status after disconnect %s", tun.Status())
	}
	if _, err := tun.Stats(); err == nil {
		t.Fatal("stats after disconnect")
	}
}

func sampleTunnelConfig(t *testing.T) *config.Config {
	t.Helper()
	client, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		PrivateKey: crypto.KeyToBase64(client.Private),
		Address:    "10.8.0.2/24",
		DNS:        []string{"1.1.1.1"},
		MTU:        1280,
		TUNName:    "wg0",
		Peers: []config.Peer{{
			PublicKey:           crypto.KeyToBase64(server.Public),
			Endpoint:            "127.0.0.1:51820",
			AllowedIPs:          []string{"10.8.0.0/24"},
			PersistentKeepalive: 25,
		}},
	}
}

// recordingTUN is only used to observe Close on the error path.
type recordingTUN struct {
	closed bool
}

func (r *recordingTUN) File() *os.File { return nil }
func (r *recordingTUN) Read([][]byte, []int, int) (int, error) {
	return 0, io.EOF
}
func (r *recordingTUN) Write(bufs [][]byte, _ int) (int, error) { return len(bufs), nil }
func (r *recordingTUN) MTU() (int, error)                       { return 1420, nil }
func (r *recordingTUN) Name() (string, error)                   { return "rec", nil }
func (r *recordingTUN) Events() <-chan tun.Event                { return nil }
func (r *recordingTUN) Close() error {
	r.closed = true
	return nil
}
func (r *recordingTUN) BatchSize() int { return 1 }

var _ tun.Device = (*recordingTUN)(nil)
