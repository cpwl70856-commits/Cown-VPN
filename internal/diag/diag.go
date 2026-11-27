// Package diag provides best-effort connection diagnostics for mywgclient.
package diag

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/crypto"
	"github.com/user/mywgclient/internal/platform"
	"github.com/user/mywgclient/internal/tunnel"
)

// Result is one diagnostic check.
type Result struct {
	Name    string
	OK      bool
	Message string
	Fix     string // optional remediation tip
}

// Options controls optional live-tunnel checks.
type Options struct {
	Tunnel *tunnel.Tunnel // may be nil
}

// Run performs network / key / UDP reachability checks (best-effort).
func Run(cfg *config.Config) []Result {
	return RunWithOptions(cfg, Options{})
}

// RunWithOptions includes route/privilege state from a live tunnel when provided.
func RunWithOptions(cfg *config.Config, opts Options) []Result {
	var out []Result

	info := platform.Snapshot()
	privOK := info.Elevated
	privMsg := "已具备管理员/root 权限"
	privFix := ""
	if !privOK {
		privMsg = "未提权 — " + info.TipZH
		privFix = "复制提权命令并以管理员/root 重新启动：\n" + info.SudoCommand
	}
	out = append(out, Result{Name: "权限", OK: privOK, Message: privMsg, Fix: privFix})

	if info.OS == "windows" {
		fix := ""
		if !info.WintunOK {
			fix = info.WintunDetail
		}
		out = append(out, Result{Name: "Wintun", OK: info.WintunOK, Message: info.WintunDetail, Fix: fix})
	}

	ifaces, err := net.Interfaces()
	up := 0
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
				up++
			}
		}
	}
	netMsg := fmt.Sprintf("检测到 %d 个已启用网卡", up)
	netFix := ""
	if up == 0 {
		netMsg = "未检测到可用网卡，请检查网络连接或飞行模式"
		netFix = "插上网线/打开 Wi‑Fi 后重试；若刚切换网络，可断开 VPN 再连接。"
	}
	out = append(out, Result{Name: "本机网络", OK: up > 0, Message: netMsg, Fix: netFix})

	// Live route state from active tunnel session.
	if opts.Tunnel != nil {
		routes := opts.Tunnel.AppliedRoutes()
		st := opts.Tunnel.Status()
		msg := fmt.Sprintf("隧道状态=%s；%s", st, tunnel.DescribeRoutes(routes))
		ok := true
		fix := ""
		if re := opts.Tunnel.LastRouteError(); re != nil {
			ok = false
			msg += "；路由警告: " + re.Error()
			fix = "点「全局路由命令」手动执行，或提权后重连。断开时应自动清理本会话路由。"
		}
		out = append(out, Result{Name: "路由状态", OK: ok, Message: msg, Fix: fix})
	} else {
		out = append(out, Result{Name: "路由状态", OK: true, Message: "当前无活动隧道（连接后可复查已安装路由）"})
	}

	if cfg == nil {
		out = append(out, Result{Name: "配置", OK: false, Message: "尚未填写配置", Fix: "使用快速向导 / 剪贴板一键导入 / 拖放 .conf"})
		return out
	}

	priv, err := crypto.ParseKey(cfg.PrivateKey)
	if err != nil {
		out = append(out, Result{Name: "本地私钥", OK: false, Message: "私钥无效: " + err.Error(),
			Fix: "用 go run ./cmd/keygen 生成密钥，或向导中「生成本机密钥对」"})
	} else {
		derived, derr := crypto.PublicFromPrivate(priv)
		msg := "私钥有效"
		if derr == nil {
			msg = "私钥有效，派生公钥: " + crypto.KeyToBase64(derived)
		}
		out = append(out, Result{Name: "本地私钥", OK: true, Message: msg})
	}

	peer := cfg.PrimaryPeer()
	if peer == nil {
		out = append(out, Result{Name: "对端", OK: false, Message: "缺少 Peer", Fix: "配置中需包含 [Peer] 段"})
		return out
	}
	_, err = crypto.ParseKey(peer.PublicKey)
	if err != nil {
		out = append(out, Result{Name: "服务端公钥", OK: false, Message: "公钥无效: " + err.Error(),
			Fix: "确认粘贴的是服务器 Interface PublicKey，而非客户端公钥"})
	} else {
		out = append(out, Result{Name: "服务端公钥", OK: true, Message: "公钥格式正确"})
	}

	if peer.Endpoint == "" {
		out = append(out, Result{Name: "Endpoint", OK: false, Message: "未填写 Endpoint", Fix: "格式 host:port，例如 vpn.example.com:51820"})
		return out
	}
	host, port, err := net.SplitHostPort(peer.Endpoint)
	if err != nil {
		if strings.Count(peer.Endpoint, ":") == 1 {
			parts := strings.SplitN(peer.Endpoint, ":", 2)
			host, port = parts[0], parts[1]
			err = nil
		}
	}
	if err != nil {
		out = append(out, Result{Name: "Endpoint", OK: false, Message: "Endpoint 格式应为 host:port"})
		return out
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		msg := "无法解析 " + host
		if err != nil {
			msg += ": " + err.Error()
		}
		out = append(out, Result{Name: "DNS 解析", OK: false, Message: msg,
			Fix: "检查 DNS / 改用 IP 作为 Endpoint；本客户端不会自动改系统 DNS（split DNS 需系统侧配置）"})
	} else {
		var b []string
		for _, ip := range ips {
			b = append(b, ip.String())
		}
		out = append(out, Result{Name: "DNS 解析", OK: true, Message: host + " → " + strings.Join(b, ", ")})
	}

	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("udp", addr, 3*time.Second)
	if err != nil {
		out = append(out, Result{Name: "UDP 可达性", OK: false, Message: "无法 dial " + addr + ": " + err.Error(),
			Fix: "检查本机网络、防火墙、服务器是否在线；公司网可能拦截 UDP"})
		return out
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write([]byte{0x01})
	buf := make([]byte, 1)
	_, rerr := conn.Read(buf)
	_ = conn.Close()
	msg := "已向 " + addr + " 发送 UDP 探测（无应答也属正常，仅表示本机可发出报文）"
	fix := "若连接后长期「尚无握手」，多半是服务器静默、密钥不匹配或端口错误。可点「测试配置」做无路由握手验证。"
	if rerr == nil {
		msg = "收到来自 " + addr + " 的 UDP 应答"
		fix = ""
	}
	out = append(out, Result{Name: "UDP 可达性", OK: true, Message: msg, Fix: fix})
	return out
}

// Format returns a multi-line Chinese summary with fix suggestions.
func Format(results []Result) string {
	var b strings.Builder
	b.WriteString("—— 连接诊断 ——\n")
	for _, r := range results {
		mark := "✗"
		if r.OK {
			mark = "✓"
		}
		fmt.Fprintf(&b, "%s [%s] %s\n", mark, r.Name, r.Message)
		if r.Fix != "" {
			fmt.Fprintf(&b, "   → 建议: %s\n", r.Fix)
		}
	}
	return b.String()
}
