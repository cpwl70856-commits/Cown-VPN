package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuickSetupDefaults(t *testing.T) {
	cfg := QuickSetup("1.2.3.4:51820", "pub", "priv")
	if cfg.Address != "10.8.0.2/24" {
		t.Fatalf("addr=%s", cfg.Address)
	}
	p := cfg.PrimaryPeer()
	if p.Endpoint != "1.2.3.4:51820" || len(p.AllowedIPs) != 1 {
		t.Fatalf("peer=%+v", p)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	s := DefaultSettings()
	s.WizardCompleted = true
	s.OnboardingDone = true
	s.RouteMode = "global"
	s.AutoReconnect = true
	if err := SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !got.WizardCompleted || !got.OnboardingDone || got.RouteMode != "global" || !got.AutoReconnect {
		t.Fatalf("%+v", got)
	}
	path, _ := SettingsPath()
	if filepath.Dir(path) != filepath.Join(dir, AppName) {
		t.Fatalf("path=%s", path)
	}
	_ = os.Remove(path)
}
