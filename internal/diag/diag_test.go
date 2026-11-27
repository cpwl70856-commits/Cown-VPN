package diag

import (
	"strings"
	"testing"

	"github.com/user/mywgclient/internal/config"
)

func TestRunNilConfig(t *testing.T) {
	results := Run(nil)
	if len(results) == 0 {
		t.Fatal("expected some results")
	}
	text := Format(results)
	if !strings.Contains(text, "连接诊断") {
		t.Fatalf("format=%s", text)
	}
}

func TestRunWithPartialConfig(t *testing.T) {
	cfg := config.FromForm(
		"YCV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=",
		"10.8.0.2/24",
		"127.0.0.1:1",
		"WGV5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2V5ZjQ0a2U=",
		"10.8.0.0/24",
		"",
		25, 0, 1420,
	)
	results := RunWithOptions(cfg, Options{})
	text := Format(results)
	if !strings.Contains(text, "本地私钥") || !strings.Contains(text, "权限") {
		t.Fatalf("unexpected: %s", text)
	}
}
