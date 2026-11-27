package platform

import (
	"runtime"
	"strings"
	"testing"
)

func TestRebuildElevatedCommandHasPath(t *testing.T) {
	cmd := RebuildElevatedCommand()
	if strings.TrimSpace(cmd) == "" {
		t.Fatal("empty command")
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(cmd, "Start-Process") {
			t.Fatalf("want powershell elevation, got %s", cmd)
		}
	default:
		if !strings.HasPrefix(cmd, "sudo ") {
			t.Fatalf("want sudo prefix, got %s", cmd)
		}
	}
}

func TestSnapshot(t *testing.T) {
	info := Snapshot()
	if info.OS != runtime.GOOS {
		t.Fatalf("os=%s", info.OS)
	}
	if info.SudoCommand == "" {
		t.Fatal("missing sudo command")
	}
}

func TestAutostartRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux autostart file test")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	off := SetAutostart(false)
	if !off.OK {
		t.Fatalf("disable: %+v", off)
	}
	on := SetAutostart(true)
	if !on.OK || !on.Enabled {
		t.Fatalf("enable: %+v", on)
	}
	st := AutostartStatus()
	if !st.Enabled {
		t.Fatalf("status: %+v", st)
	}
	off2 := SetAutostart(false)
	if !off2.OK || off2.Enabled {
		t.Fatalf("disable2: %+v", off2)
	}
}
