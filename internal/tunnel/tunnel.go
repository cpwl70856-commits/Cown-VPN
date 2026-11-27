// Package tunnel wraps wireguard-go Device for the mywgclient desktop app.
package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/crypto"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// Status describes the connection lifecycle shown in the UI.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusTesting      Status = "testing"
)

// Stats holds traffic counters from the WireGuard UAPI.
type Stats struct {
	RxBytes           int64
	TxBytes           int64
	LastHandshakeUnix int64
}

// RateStats is Stats plus a simple instantaneous rate between samples.
type RateStats struct {
	Stats
	RxPerSec float64
	TxPerSec float64
}

// ConnectOptions controls routing behaviour for a Connect call.
type ConnectOptions struct {
	RouteMode RouteMode
	// SkipRoutes forces no OS route installation (same as RouteModeNone).
	SkipRoutes bool
}

// Tunnel owns a wireguard-go Device and its underlying TUN.
type Tunnel struct {
	mu            sync.Mutex
	dev           *device.Device
	tunDev        tun.Device
	name          string
	addr          string
	status        Status
	cancel        context.CancelFunc
	appliedRoutes []AppliedRoute
	routeMode     RouteMode
	lastRouteErr  error
	connectedAt   time.Time
	connectGen    uint64 // bumped on cancel / disconnect to invalidate in-flight Connect

	// rate sampling
	prevRx   int64
	prevTx   int64
	prevAt   time.Time
	rxPerSec float64
	txPerSec float64
}

// New returns a disconnected tunnel handle.
func New() *Tunnel {
	t := &Tunnel{status: StatusDisconnected, routeMode: RouteModeSplit}
	RegisterForCleanup(t)
	return t
}

// Status returns the current connection status.
func (t *Tunnel) Status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Name returns the OS interface name (empty if not up).
func (t *Tunnel) Name() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.name
}

// Address returns the configured virtual IP / CIDR.
func (t *Tunnel) Address() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.addr
}

// ConnectedAt returns when the tunnel last reached connected state (zero if not connected).
func (t *Tunnel) ConnectedAt() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connectedAt
}

// LastRouteError returns the last best-effort routing error (may be non-nil even when connected).
func (t *Tunnel) LastRouteError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastRouteErr
}

// RouteMode returns the routing mode used for the current/last session.
func (t *Tunnel) RouteMode() RouteMode {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.routeMode
}

// AppliedRoutes returns a copy of routes installed for this session.
func (t *Tunnel) AppliedRoutes() []AppliedRoute {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]AppliedRoute, len(t.appliedRoutes))
	copy(out, t.appliedRoutes)
	return out
}

// Connect creates a TUN, applies UAPI config, brings the device up, and assigns the address.
func (t *Tunnel) Connect(cfg *config.Config, timeout time.Duration) error {
	return t.ConnectWithOptions(cfg, timeout, ConnectOptions{RouteMode: RouteModeSplit})
}

// ConnectWithOptions is like Connect but honours routing mode (分流 / 全局 / 无路由).
func (t *Tunnel) ConnectWithOptions(cfg *config.Config, timeout time.Duration, opts ConnectOptions) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	mode := opts.RouteMode
	if opts.SkipRoutes {
		mode = RouteModeNone
	}
	if mode == "" {
		mode = RouteModeSplit
	}

	t.mu.Lock()
	if t.status == StatusConnected || t.status == StatusConnecting || t.status == StatusTesting {
		st := t.status
		t.mu.Unlock()
		return fmt.Errorf("已处于 %s 状态，请先断开", st)
	}
	t.connectGen++
	gen := t.connectGen
	t.status = StatusConnecting
	t.routeMode = mode
	t.lastRouteErr = nil
	t.connectedAt = time.Time{}
	t.prevRx, t.prevTx = 0, 0
	t.prevAt = time.Time{}
	t.rxPerSec, t.txPerSec = 0, 0
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		err error
	}
	errCh := make(chan result, 1)
	go func() {
		errCh <- result{err: t.start(cfg, mode, gen)}
	}()

	select {
	case res := <-errCh:
		if res.err != nil {
			t.setStatusIfGen(gen, StatusDisconnected)
			return res.err
		}
		if !t.setStatusIfGen(gen, StatusConnected) {
			_ = t.Disconnect()
			return fmt.Errorf("连接已取消")
		}
		return nil
	case <-ctx.Done():
		_ = t.Disconnect()
		return fmt.Errorf("连接超时（%s）：服务器无响应或网络不可达", timeout)
	}
}

// TestHandshake brings WireGuard up WITHOUT installing 0.0.0.0/0 (or any) OS routes,
// waits until a peer handshake appears (or timeout), then tears everything down.
// Safe to run while the host should keep normal networking.
func (t *Tunnel) TestHandshake(cfg *config.Config, timeout time.Duration) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if peer := cfg.PrimaryPeer(); peer == nil || peer.Endpoint == "" {
		return fmt.Errorf("测试配置需要填写服务器 Endpoint")
	}

	t.mu.Lock()
	if t.status == StatusConnected || t.status == StatusConnecting || t.status == StatusTesting {
		st := t.status
		t.mu.Unlock()
		return fmt.Errorf("已处于 %s 状态，请先断开后再测试", st)
	}
	t.connectGen++
	gen := t.connectGen
	t.status = StatusTesting
	t.routeMode = RouteModeNone
	t.lastRouteErr = nil
	t.connectedAt = time.Time{}
	t.mu.Unlock()

	defer func() {
		_ = t.Disconnect()
	}()

	if err := t.start(cfg, RouteModeNone, gen); err != nil {
		t.setStatusIfGen(gen, StatusDisconnected)
		return err
	}
	if !t.setStatusIfGen(gen, StatusTesting) {
		return fmt.Errorf("测试已取消")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t.mu.Lock()
		cancelled := t.connectGen != gen
		t.mu.Unlock()
		if cancelled {
			return fmt.Errorf("测试已取消")
		}
		st, err := t.Stats()
		if err == nil && st.LastHandshakeUnix > 0 {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("握手超时（%s）：服务器静默或密钥/Endpoint 不匹配。隧道已拆除且未改路由", timeout)
}

func (t *Tunnel) start(cfg *config.Config, mode RouteMode, gen uint64) error {
	t.mu.Lock()
	cancelled := t.connectGen != gen
	t.mu.Unlock()
	if cancelled {
		return fmt.Errorf("连接已取消")
	}

	name := cfg.TUNName
	if name == "" {
		name = "wg0"
	}
	if runtime.GOOS == "darwin" {
		name = "utun"
	}

	tunDev, err := tun.CreateTUN(name, cfg.MTU)
	if err != nil {
		return fmt.Errorf("create TUN: %w (need root/admin privileges; on Windows install Wintun)", err)
	}
	realName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return fmt.Errorf("tun name: %w", err)
	}

	// Abort early if user cancelled while CreateTUN was blocking.
	t.mu.Lock()
	if t.connectGen != gen {
		t.mu.Unlock()
		tunDev.Close()
		return fmt.Errorf("连接已取消")
	}
	t.mu.Unlock()

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	bind := conn.NewDefaultBind()
	dev := device.NewDevice(tunDev, bind, logger)

	uapiCfg := cfg
	if mode == RouteModeGlobal {
		// Cryptokey routing: tell wireguard-go all IPv4/IPv6 goes to the peer.
		// OS default route is installed separately as half-routes for safe cleanup.
		cloned := *cfg
		cloned.Peers = append([]config.Peer(nil), cfg.Peers...)
		for i := range cloned.Peers {
			cloned.Peers[i].AllowedIPs = []string{"0.0.0.0/0", "::/0"}
		}
		uapiCfg = &cloned
	}

	uapi, err := buildUAPI(uapiCfg)
	if err != nil {
		dev.Close()
		return err
	}
	slog.Debug("wireguard uapi", "config", redactUAPI(uapi))

	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return fmt.Errorf("IpcSet: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return fmt.Errorf("device up: %w", err)
	}

	if err := configureAddress(realName, cfg.Address); err != nil {
		dev.Close()
		return fmt.Errorf("configure address: %w", err)
	}

	var routeErr error
	var applied []AppliedRoute
	if mode != RouteModeNone {
		if peer := cfg.PrimaryPeer(); peer != nil {
			dests, _ := expandAllowedForMode(peer.AllowedIPs, mode)
			applied, routeErr = applyRoutes(realName, dests, cfg.Address)
			if routeErr != nil {
				slog.Warn("route setup incomplete", "err", routeErr)
			}
		}
	}

	t.mu.Lock()
	if t.connectGen != gen {
		t.mu.Unlock()
		if len(applied) > 0 {
			removeRoutes(applied)
		}
		dev.Close()
		return fmt.Errorf("连接已取消")
	}
	t.dev = dev
	t.tunDev = tunDev
	t.name = realName
	t.addr = cfg.Address
	t.appliedRoutes = applied
	t.lastRouteErr = routeErr
	t.connectedAt = time.Now()
	t.mu.Unlock()

	slog.Info("wireguard tunnel up", "iface", realName, "addr", cfg.Address, "os", runtime.GOOS, "routes", len(applied), "mode", mode)
	return nil
}

// UpFromDevice runs wireguard-go on a TUN device the caller already created.
// Android VpnService owns the address, DNS, and routes; this method does not
// configure them. On success the Tunnel owns tunDev and closes it from
// Disconnect. On failure tunDev is closed. A nil bind uses the platform default.
func (t *Tunnel) UpFromDevice(cfg *config.Config, tunDev tun.Device, bind conn.Bind) error {
	if tunDev == nil {
		return fmt.Errorf("TUN 设备为空")
	}
	if err := cfg.Validate(); err != nil {
		_ = tunDev.Close()
		return err
	}
	if bind == nil {
		bind = conn.NewDefaultBind()
	}

	t.mu.Lock()
	if t.status == StatusConnected || t.status == StatusConnecting || t.status == StatusTesting {
		st := t.status
		t.mu.Unlock()
		_ = tunDev.Close()
		return fmt.Errorf("已处于 %s 状态，请先断开", st)
	}
	t.connectGen++
	gen := t.connectGen
	t.status = StatusConnecting
	t.routeMode = RouteModeNone
	t.lastRouteErr = nil
	t.connectedAt = time.Time{}
	t.appliedRoutes = nil
	t.prevRx, t.prevTx = 0, 0
	t.prevAt = time.Time{}
	t.rxPerSec, t.txPerSec = 0, 0
	t.mu.Unlock()

	fail := func(err error) error {
		t.setStatusIfGen(gen, StatusDisconnected)
		return err
	}

	realName, err := tunDev.Name()
	if err != nil || realName == "" {
		realName = cfg.TUNName
		if realName == "" {
			realName = "wg0"
		}
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	dev := device.NewDevice(tunDev, bind, logger)
	// Before peers exist. Android networks change address without a new endpoint.
	dev.DisableSomeRoamingForBrokenMobileSemantics()

	uapi, err := buildUAPI(cfg)
	if err != nil {
		dev.Close()
		return fail(err)
	}
	slog.Debug("wireguard uapi", "config", redactUAPI(uapi))
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return fail(fmt.Errorf("IpcSet: %w", err))
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return fail(fmt.Errorf("device up: %w", err))
	}

	t.mu.Lock()
	if t.connectGen != gen {
		t.mu.Unlock()
		dev.Close()
		return fmt.Errorf("连接已取消")
	}
	t.dev = dev
	t.tunDev = tunDev
	t.name = realName
	t.addr = cfg.Address
	t.appliedRoutes = nil
	t.connectedAt = time.Now()
	t.status = StatusConnected
	t.mu.Unlock()
	slog.Info("wireguard tunnel up from existing TUN", "iface", realName, "addr", cfg.Address, "os", runtime.GOOS)
	return nil
}

// Disconnect tears down routes, WireGuard, and the TUN device.
// Critical: always remove tracked routes first so a failed global session
// cannot leave 0.0.0.0/1 half-routes that break host networking.
func (t *Tunnel) Disconnect() error {
	t.mu.Lock()
	t.connectGen++ // invalidate any in-flight Connect / TestHandshake
	routes := append([]AppliedRoute(nil), t.appliedRoutes...)
	t.appliedRoutes = nil
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	dev := t.dev
	name := t.name
	t.dev = nil
	t.tunDev = nil
	t.name = ""
	t.addr = ""
	t.connectedAt = time.Time{}
	t.status = StatusDisconnected
	t.prevRx, t.prevTx = 0, 0
	t.prevAt = time.Time{}
	t.rxPerSec, t.txPerSec = 0, 0
	t.mu.Unlock()

	if len(routes) > 0 {
		slog.Info("cleaning up routes", "count", len(routes), "iface", name)
		if n := removeRoutes(routes); n > 0 {
			slog.Warn("部分路由清理失败", "failed", n, "total", len(routes))
			// Best-effort second pass after a short delay (Windows/netsh races).
			time.Sleep(100 * time.Millisecond)
			_ = removeRoutes(routes)
		}
	}
	if dev != nil {
		slog.Info("shutting down wireguard tunnel", "iface", name)
		dev.Close()
	}
	return nil
}

// Wait returns a channel closed when the device shuts down, or nil if not connected.
func (t *Tunnel) Wait() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dev == nil {
		return nil
	}
	return t.dev.Wait()
}

// Stats reads rx/tx bytes and last handshake from UAPI get.
func (t *Tunnel) Stats() (Stats, error) {
	t.mu.Lock()
	dev := t.dev
	t.mu.Unlock()
	if dev == nil {
		return Stats{}, fmt.Errorf("未连接")
	}
	raw, err := dev.IpcGet()
	if err != nil {
		return Stats{}, err
	}
	return parseStats(raw), nil
}

// StatsWithRate reads counters and updates a simple bytes/s estimate.
func (t *Tunnel) StatsWithRate() (RateStats, error) {
	st, err := t.Stats()
	if err != nil {
		return RateStats{}, err
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	rs := RateStats{Stats: st}
	if !t.prevAt.IsZero() {
		dt := now.Sub(t.prevAt).Seconds()
		if dt > 0.2 {
			t.rxPerSec = float64(st.RxBytes-t.prevRx) / dt
			t.txPerSec = float64(st.TxBytes-t.prevTx) / dt
			if t.rxPerSec < 0 {
				t.rxPerSec = 0
			}
			if t.txPerSec < 0 {
				t.txPerSec = 0
			}
		}
	}
	t.prevRx = st.RxBytes
	t.prevTx = st.TxBytes
	t.prevAt = now
	rs.RxPerSec = t.rxPerSec
	rs.TxPerSec = t.txPerSec
	return rs, nil
}

// TestTUN tries to create and immediately destroy a TUN device to surface permission issues.
func TestTUN(mtu int) (ifaceName string, err error) {
	return testCreateTUN(mtu)
}

// BuildUAPIString exposes the WireGuard UAPI config for mobile / extension bridges.
func BuildUAPIString(cfg *config.Config) (string, error) {
	return buildUAPI(cfg)
}

func (t *Tunnel) setStatus(s Status) {
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
}

func (t *Tunnel) setStatusIfGen(gen uint64, s Status) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.connectGen != gen {
		return false
	}
	t.status = s
	return true
}

func buildUAPI(cfg *config.Config) (string, error) {
	priv, err := crypto.ParseKey(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("private_key: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", crypto.KeyToHex(priv))
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&b, "listen_port=%d\n", cfg.ListenPort)
	}
	fmt.Fprintf(&b, "replace_peers=true\n")

	for i, p := range cfg.Peers {
		pub, err := crypto.ParseKey(p.PublicKey)
		if err != nil {
			return "", fmt.Errorf("peers[%d].public_key: %w", i, err)
		}
		fmt.Fprintf(&b, "public_key=%s\n", crypto.KeyToHex(pub))
		if p.PresharedKey != "" {
			psk, err := crypto.ParseKey(p.PresharedKey)
			if err != nil {
				return "", fmt.Errorf("peers[%d].preshared_key: %w", i, err)
			}
			fmt.Fprintf(&b, "preshared_key=%s\n", crypto.KeyToHex(psk))
		}
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
		}
		ka := p.PersistentKeepalive
		if ka > 0 {
			fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", ka)
		}
		fmt.Fprintf(&b, "replace_allowed_ips=true\n")
		for _, ip := range p.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", ip)
		}
	}
	return b.String(), nil
}

func redactUAPI(uapi string) string {
	lines := strings.Split(uapi, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "private_key=") || strings.HasPrefix(line, "preshared_key=") {
			lines[i] = strings.Split(line, "=")[0] + "=<redacted>"
		}
	}
	return strings.Join(lines, "\n")
}

func parseStats(uapi string) Stats {
	var s Stats
	for _, line := range strings.Split(uapi, "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "rx_bytes":
			fmt.Sscanf(val, "%d", &s.RxBytes)
		case "tx_bytes":
			fmt.Sscanf(val, "%d", &s.TxBytes)
		case "last_handshake_time_sec":
			fmt.Sscanf(val, "%d", &s.LastHandshakeUnix)
		}
	}
	return s
}

// StartServer brings up a userspace WireGuard peer (server role) with a listen port.
func StartServer(cfg *config.Config) (*Tunnel, error) {
	if cfg.ListenPort <= 0 {
		cfg.ListenPort = config.DefaultListenPort
	}
	t := New()
	t.mu.Lock()
	t.connectGen++
	gen := t.connectGen
	t.status = StatusConnecting
	t.mu.Unlock()
	if err := t.start(cfg, RouteModeSplit, gen); err != nil {
		t.setStatus(StatusDisconnected)
		return nil, err
	}
	t.setStatus(StatusConnected)
	return t, nil
}
