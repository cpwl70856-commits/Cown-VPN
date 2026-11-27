package wgmobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/mywgclient/internal/crypto"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/tun"
)

func TestParseConfAndroidContract(t *testing.T) {
	client, server := testKeys(t)
	conf := confText(client, server)
	raw, err := ParseConf(conf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, client) {
		t.Fatalf("hints leaked private key: %s", raw)
	}
	var hints NetworkHints
	if err := json.Unmarshal([]byte(raw), &hints); err != nil {
		t.Fatal(err)
	}
	if hints.Address != "10.8.0.2/24" || hints.AddressIP != "10.8.0.2" {
		t.Fatalf("address %+v", hints)
	}
	if hints.MTU != 1280 {
		t.Fatalf("mtu %d", hints.MTU)
	}
	if hints.Endpoint != "127.0.0.1:51820" {
		t.Fatalf("endpoint %s", hints.Endpoint)
	}
	if len(hints.DNS) != 1 || hints.DNS[0] != "1.1.1.1" {
		t.Fatalf("dns %v", hints.DNS)
	}
	if len(hints.AllowedIPs) != 2 || hints.AllowedIPs[0] != "10.8.0.0/24" || hints.AllowedIPs[1] != "::/0" {
		t.Fatalf("allowed %v", hints.AllowedIPs)
	}
	if hints.ServerPubKey != server {
		t.Fatalf("server key %s", hints.ServerPubKey)
	}
}

func TestParseConfRejectsEmpty(t *testing.T) {
	if _, err := ParseConf("  \n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildUAPIHidesBase64Keys(t *testing.T) {
	client, server := testKeys(t)
	uapi, err := BuildUAPI(confText(client, server))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(uapi, client) || strings.Contains(uapi, server) {
		t.Fatal("UAPI should be hex, not the base64 conf keys")
	}
	if !strings.Contains(uapi, "public_key=") || !strings.Contains(uapi, "private_key=") {
		t.Fatalf("missing keys: %s", uapi)
	}
	if !strings.Contains(uapi, "endpoint=127.0.0.1:51820") {
		t.Fatalf("endpoint: %s", uapi)
	}
	if !strings.Contains(uapi, "allowed_ip=::/0") {
		t.Fatalf("allowed: %s", uapi)
	}
}

func TestStartWithFDRequiresAndroid(t *testing.T) {
	client, server := testKeys(t)
	err := StartWithFD(confText(client, server), 7)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Android") {
		t.Fatalf("error %v", err)
	}
	if Status() != "disconnected" {
		t.Fatalf("status %s", Status())
	}
	Stop()
	if SocketV4() != -1 || SocketV6() != -1 {
		t.Fatal("sockets should be closed")
	}
}

func TestProtectingBindCallsProtector(t *testing.T) {
	rec := &recordingProtector{}
	SetProtector(rec)
	t.Cleanup(ClearProtector)

	inner := &peekBind{fd4: 4, fd6: -1}
	if _, _, err := (&protectingBind{Bind: inner}).Open(0); err != nil {
		t.Fatal(err)
	}
	if len(rec.fds) != 1 || rec.fds[0] != 4 {
		t.Fatalf("fds %v", rec.fds)
	}

	ClearProtector()
	inner.fd4 = 9
	if _, _, err := (&protectingBind{Bind: inner}).Open(0); err != nil {
		t.Fatal(err)
	}
	if len(rec.fds) != 1 {
		t.Fatalf("protector cleared but got %v", rec.fds)
	}
}

func TestCallPeekRecovers(t *testing.T) {
	got := callPeek(func() (int, error) {
		panic("ipv6 socket missing")
	})
	if got != -1 {
		t.Fatalf("fd %d", got)
	}
}

func TestFixedMTU(t *testing.T) {
	d := &fixedMTU{Device: stubTUN{}, mtu: 1280}
	mtu, err := d.MTU()
	if err != nil || mtu != 1280 {
		t.Fatalf("mtu=%d err=%v", mtu, err)
	}
	if d.BatchSize() != 1 {
		t.Fatalf("batch %d", d.BatchSize())
	}
	d.mtu = 0
	mtu, err = d.MTU()
	if err != nil || mtu != 100 {
		t.Fatalf("fallback mtu=%d err=%v", mtu, err)
	}
}

func TestBuildScriptDocumentsRequiredEnv(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "android", "scripts", "build_aar.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("build_aar.sh is not executable")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, needle := range []string{
		"ANDROID_HOME",
		"ANDROID_NDK_HOME",
		"gomobile",
		"android/app/libs/wgmobile.aar",
		"github.com/user/mywgclient/pkg/wgmobile",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("script missing %s", needle)
		}
	}
}

type recordingProtector struct {
	fds []int
}

func (r *recordingProtector) Protect(fd int) bool {
	r.fds = append(r.fds, fd)
	return true
}

type peekBind struct {
	fd4, fd6 int
}

func (p *peekBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	return nil, port, nil
}
func (p *peekBind) Close() error                                { return nil }
func (p *peekBind) SetMark(uint32) error                        { return nil }
func (p *peekBind) Send([][]byte, conn.Endpoint) error          { return nil }
func (p *peekBind) ParseEndpoint(string) (conn.Endpoint, error) { return nil, nil }
func (p *peekBind) BatchSize() int                              { return 1 }
func (p *peekBind) PeekLookAtSocketFd4() (int, error)           { return p.fd4, nil }
func (p *peekBind) PeekLookAtSocketFd6() (int, error)           { return -1, errNoTUN }

type stubTUN struct{ tun.Device }

func (stubTUN) MTU() (int, error) { return 100, nil }
func (stubTUN) BatchSize() int    { return 0 }

func testKeys(t *testing.T) (clientPriv, serverPub string) {
	t.Helper()
	client, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return crypto.KeyToBase64(client.Private), crypto.KeyToBase64(server.Public)
}

func confText(clientPriv, serverPub string) string {
	return "[Interface]\nPrivateKey = " + clientPriv + "\nAddress = 10.8.0.2/24\nDNS = 1.1.1.1\nMTU = 1280\n\n[Peer]\nPublicKey = " + serverPub + "\nEndpoint = 127.0.0.1:51820\nAllowedIPs = 10.8.0.0/24, ::/0\nPersistentKeepalive = 25\n"
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
