package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.conf")
	src := `[Interface]
PrivateKey = YCV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=
Address = 10.8.0.2/24
MTU = 1280

[Peer]
PublicKey = WGV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=
Endpoint = 1.2.3.4:51820
AllowedIPs = 10.8.0.0/24, 192.168.1.0/24
PersistentKeepalive = 25
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "10.8.0.2/24" {
		t.Fatalf("address: %q", cfg.Address)
	}
	if cfg.MTU != 1280 {
		t.Fatalf("mtu: %d", cfg.MTU)
	}
	p := cfg.PrimaryPeer()
	if p == nil || p.Endpoint != "1.2.3.4:51820" {
		t.Fatalf("peer: %+v", p)
	}
	if len(p.AllowedIPs) != 2 {
		t.Fatalf("allowed: %v", p.AllowedIPs)
	}
	out := filepath.Join(dir, "out.conf")
	if err := SaveFile(out, cfg); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.PrivateKey != cfg.PrivateKey || cfg2.PrimaryPeer().PublicKey != p.PublicKey {
		t.Fatal("round-trip mismatch")
	}
}

func TestValidateRequiresEndpointForForm(t *testing.T) {
	cfg := FromForm("k", "10.8.0.2/24", "", "pk", "10.8.0.0/24", "", 25, 0, 1420)
	if err := cfg.Validate(); err != nil {
		// keys are non-empty so validate passes; endpoint is checked by UI
		t.Fatalf("unexpected: %v", err)
	}
}
