package config

import (
	"encoding/base64"
	"testing"
)

const sampleConf = `[Interface]
PrivateKey = YCV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=
Address = 10.8.0.2/24
MTU = 1280

[Peer]
PublicKey = WGV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=
Endpoint = 1.2.3.4:51820
AllowedIPs = 10.8.0.0/24
PersistentKeepalive = 25
`

func TestLoadString(t *testing.T) {
	cfg, err := LoadString(sampleConf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "10.8.0.2/24" || cfg.PrimaryPeer().Endpoint != "1.2.3.4:51820" {
		t.Fatalf("%+v", cfg)
	}
}

func TestParseImportTextPlainAndVPN(t *testing.T) {
	cfg, err := ParseImportText(sampleConf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrimaryPeer().Endpoint != "1.2.3.4:51820" {
		t.Fatal(cfg.PrimaryPeer().Endpoint)
	}
	cfg2, err := ParseImportText("vpn://" + sampleConf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Address != cfg.Address {
		t.Fatal("vpn:// mismatch")
	}
	enc := base64.StdEncoding.EncodeToString([]byte(sampleConf))
	cfg3, err := ParseImportText("vpn://" + enc)
	if err != nil {
		t.Fatal(err)
	}
	if cfg3.PrimaryPeer().PublicKey != cfg.PrimaryPeer().PublicKey {
		t.Fatal("base64 vpn mismatch")
	}
}

func TestParseImportTextEmpty(t *testing.T) {
	if _, err := ParseImportText("   "); err == nil {
		t.Fatal("expected error")
	}
}
