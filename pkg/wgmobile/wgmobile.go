// Package wgmobile exposes a tiny gomobile-friendly WireGuard API for mobile clients.
//
// iOS (on macOS with Xcode + gomobile):
//
//	go install golang.org/x/mobile/cmd/gomobile@latest
//	gomobile init
//	gomobile bind -target=ios -o WGMobile.xcframework github.com/user/mywgclient/pkg/wgmobile
//
// The Android app under android/ does not link this package. It depends on
// com.wireguard.android:tunnel and starts the tunnel with GoBackend.setState.
// android/scripts/build_aar.sh can still bind this package to an AAR, but that
// file is not required to build or run the Android app.
//
// On iOS you generally cannot call CreateTUN from a normal process. Prefer
// ParseConf / BuildUAPI from the Packet Tunnel provider together with packet
// I/O bridged through NEPacketTunnelFlow (or WireGuardKit). Start/Stop below
// use wireguard-go userspace when the host provides a usable TUN (desktop /
// some extension setups); treat them as a thin control plane for status and UAPI.
package wgmobile

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/tunnel"

	"golang.zx2c4.com/wireguard/conn"
)

var (
	mu         sync.Mutex
	active     *tunnel.Tunnel
	lastCfg    string
	protector  Protector
	activeBind conn.Bind
)

// Protector is implemented by Android VpnService so WireGuard's UDP sockets
// are excluded from the tunnel. Without it, handshake packets loop into the VPN.
// gomobile maps this to wgmobile.Protector.protect(long).
type Protector interface {
	Protect(fd int) bool
}

// TunnelStatus is a JSON-serializable snapshot for Swift.
type TunnelStatus struct {
	State             string `json:"state"` // disconnected|connecting|connected
	Address           string `json:"address"`
	Iface             string `json:"iface"`
	RxBytes           int64  `json:"rx_bytes"`
	TxBytes           int64  `json:"tx_bytes"`
	LastHandshakeUnix int64  `json:"last_handshake_unix"`
	ConnectedAtUnix   int64  `json:"connected_at_unix"`
	Error             string `json:"error,omitempty"`
}

// NetworkHints helps Swift build NEPacketTunnelNetworkSettings.
type NetworkHints struct {
	Address      string   `json:"address"`
	AddressIP    string   `json:"address_ip"`
	DNS          []string `json:"dns"`
	MTU          int      `json:"mtu"`
	Endpoint     string   `json:"endpoint"`
	AllowedIPs   []string `json:"allowed_ips"`
	ServerPubKey string   `json:"server_public_key"`
}

// ParseConf validates WireGuard conf text and returns JSON NetworkHints.
func ParseConf(confText string) (string, error) {
	cfg, err := config.ParseImportText(confText)
	if err != nil {
		return "", err
	}
	hints := NetworkHints{
		Address:   cfg.Address,
		AddressIP: cfg.VirtualIP(),
		DNS:       cfg.DNS,
		MTU:       cfg.MTU,
	}
	if p := cfg.PrimaryPeer(); p != nil {
		hints.Endpoint = p.Endpoint
		hints.AllowedIPs = p.AllowedIPs
		hints.ServerPubKey = p.PublicKey
	}
	b, err := json.Marshal(hints)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildUAPI converts WireGuard conf text into a wireguard-go IpcSet string.
func BuildUAPI(confText string) (string, error) {
	cfg, err := config.ParseImportText(confText)
	if err != nil {
		return "", err
	}
	return tunnel.BuildUAPIString(cfg)
}

// Start brings up a userspace WireGuard tunnel from conf text (desktop / test).
// On iOS Network Extension, prefer ParseConf + BuildUAPI and drive packets via NE.
// On Android, use StartWithFD with the VpnService TUN fd instead of Start.
func Start(confText string) error {
	cfg, err := config.ParseImportText(confText)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if active != nil && active.Status() == tunnel.StatusConnected {
		return fmt.Errorf("隧道已连接，请先 Stop")
	}
	if active != nil {
		_ = active.Disconnect()
		active = nil
	}
	t := tunnel.New()
	if err := t.Connect(cfg, 25*time.Second); err != nil {
		return err
	}
	active = t
	activeBind = nil
	lastCfg = confText
	return nil
}

// SetProtector registers the Android VpnService.protect hook. Pass nil to clear.
// Call it before StartWithFD. Safe to call from any goroutine.
func SetProtector(p Protector) {
	mu.Lock()
	protector = p
	mu.Unlock()
}

// ClearProtector removes the hook registered by SetProtector.
func ClearProtector() { SetProtector(nil) }

// StartWithFD brings up userspace WireGuard on an existing TUN fd (Android VpnService).
// The fd is duplicated; the caller may close the original after this call returns.
// Address, DNS, and routes are left to the caller. SetProtector should already
// be set so the UDP sockets can be excluded from the VPN. SocketV4 and SocketV6
// expose those fds for a second protect pass after this returns.
func StartWithFD(confText string, tunFd int) error {
	cfg, err := config.ParseImportText(confText)
	if err != nil {
		return err
	}
	mu.Lock()
	busy := active != nil
	mu.Unlock()
	if busy {
		return fmt.Errorf("隧道已连接，请先 Stop")
	}

	tunDev, bind, err := adoptAndroidTUN(cfg, tunFd)
	if err != nil {
		return err
	}
	t := tunnel.New()
	if err := t.UpFromDevice(cfg, &fixedMTU{Device: tunDev, mtu: cfg.MTU}, &protectingBind{Bind: bind}); err != nil {
		return err
	}

	mu.Lock()
	if active != nil {
		mu.Unlock()
		_ = t.Disconnect()
		return fmt.Errorf("隧道已连接，请先 Stop")
	}
	active = t
	activeBind = bind
	lastCfg = confText
	mu.Unlock()
	return nil
}

// Stop tears down the active tunnel and cleans routes when applicable.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	activeBind = nil
	if active != nil {
		_ = active.Disconnect()
		active = nil
	}
}

// Status returns a short state string: disconnected|connecting|connected.
func Status() string {
	mu.Lock()
	defer mu.Unlock()
	if active == nil {
		return string(tunnel.StatusDisconnected)
	}
	return string(active.Status())
}

// StatsJSON returns a TunnelStatus JSON blob (rx/tx/handshake).
func StatsJSON() string {
	mu.Lock()
	defer mu.Unlock()
	st := TunnelStatus{State: string(tunnel.StatusDisconnected)}
	if active == nil {
		b, _ := json.Marshal(st)
		return string(b)
	}
	st.State = string(active.Status())
	st.Address = active.Address()
	st.Iface = active.Name()
	if !active.ConnectedAt().IsZero() {
		st.ConnectedAtUnix = active.ConnectedAt().Unix()
	}
	if stats, err := active.Stats(); err == nil {
		st.RxBytes = stats.RxBytes
		st.TxBytes = stats.TxBytes
		st.LastHandshakeUnix = stats.LastHandshakeUnix
	} else if err != nil {
		st.Error = err.Error()
	}
	b, _ := json.Marshal(st)
	return string(b)
}

// LastConfig returns the conf text used by the last successful Start (may be empty).
func LastConfig() string {
	mu.Lock()
	defer mu.Unlock()
	return lastCfg
}
