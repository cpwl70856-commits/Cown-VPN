package errs

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyTUN(t *testing.T) {
	msg := Friendly(errors.New("create TUN: operation not permitted"))
	if !strings.Contains(msg, "TUN") {
		t.Fatalf("msg=%s", msg)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "sudo") || !strings.Contains(lower, "wintun") {
		t.Fatalf("missing remediation: %s", msg)
	}
}

func TestFriendlyKey(t *testing.T) {
	msg := Friendly(errors.New("private_key: base64 key: illegal"))
	if !strings.Contains(msg, "密钥") {
		t.Fatalf("msg=%s", msg)
	}
}

func TestFriendlyHandshake(t *testing.T) {
	msg := Friendly(errors.New("握手超时（15s）：服务器静默或密钥/Endpoint 不匹配"))
	if !strings.Contains(msg, "握手") && !strings.Contains(msg, "静默") {
		t.Fatalf("msg=%s", msg)
	}
}

func TestFriendlyDoubleConnect(t *testing.T) {
	msg := Friendly(errors.New("已处于 connecting 状态，请先断开"))
	if !strings.Contains(msg, "状态冲突") {
		t.Fatalf("msg=%s", msg)
	}
}
