// Package errs maps common tunnel/config failures to Chinese remediation tips.
package errs

import (
	"errors"
	"fmt"
	"strings"
)

// Friendly wraps an error with a Chinese headline and remediation tip.
func Friendly(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "create tun"), strings.Contains(lower, "operation not permitted"),
		strings.Contains(lower, "access is denied"), strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "wintun"):
		return fmt.Sprintf("无法创建 TUN 设备：%v\n\n排查建议：\n"+
			"• Linux/macOS：使用 sudo 或以 root 运行（可在界面复制提权命令，含完整路径）\n"+
			"• Windows：以管理员运行，并确保 wintun.dll 在程序目录（https://www.wintun.net）\n"+
			"• 可先点击「测试 TUN」确认权限", err)

	case strings.Contains(lower, "private_key"), strings.Contains(lower, "public_key"),
		strings.Contains(lower, "preshared_key"), strings.Contains(lower, "base64 key"),
		strings.Contains(lower, "hex key"), strings.Contains(lower, "key must be"),
		strings.Contains(msg, "密钥"):
		return fmt.Sprintf("密钥无效：%v\n\n请使用 go run ./cmd/keygen 生成标准 WireGuard base64 密钥（约 44 字符），"+
			"并确认客户端 PrivateKey 与服务端 Peer PublicKey 配对正确。密钥错误时服务器通常保持静默。", err)

	case strings.Contains(lower, "握手超时"), strings.Contains(lower, "服务器静默"),
		strings.Contains(lower, "尚无握手"):
		return fmt.Sprintf("服务器无握手响应：%v\n\n排查建议：\n"+
			"• Endpoint 主机/端口是否正确，防火墙是否放行 UDP\n"+
			"• 客户端公钥是否已加入服务器 [Peer]\n"+
			"• 密钥是否配对；可用「测试配置」验证（不改系统路由）\n"+
			"• 使用「连接诊断」检查 DNS / UDP", err)

	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"),
		strings.Contains(msg, "连接超时"):
		return fmt.Sprintf("连接超时：%v\n\n排查建议：\n"+
			"• 检查 Endpoint 主机名/端口是否正确\n"+
			"• 确认服务器已启动且防火墙放行 UDP\n"+
			"• 服务器静默（密钥错误）时也会表现为超时\n"+
			"• 使用「连接诊断」或「测试配置」", err)

	case strings.Contains(lower, "address already in use"), strings.Contains(lower, "bind:"),
		strings.Contains(lower, "listen_port"),
		(strings.Contains(lower, "port") && strings.Contains(lower, "in use")):
		return fmt.Sprintf("端口可能被占用：%v\n\n请将 Listen Port 留空（由系统分配），或关闭占用该 UDP 端口的进程后重试。", err)

	case strings.Contains(lower, "route"), strings.Contains(lower, "netsh"),
		strings.Contains(lower, "ip route"), strings.Contains(lower, "ifconfig"):
		return fmt.Sprintf("路由/地址配置失败：%v\n\n通常需要管理员权限。Linux 使用 `ip`，macOS 使用 `route`/`ifconfig`，Windows 使用 `netsh`。"+
			"断开连接时会清理本会话添加的路由；若主机网络异常，请点「全局路由命令」查看清理命令。", err)

	case strings.Contains(lower, "network is unreachable"), strings.Contains(lower, "no such host"),
		strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "network changed"), strings.Contains(msg, "网络"):
		return fmt.Sprintf("网络不可达或已变化：%v\n\n请检查本机网络、DNS，以及服务器是否在线。"+
			"切换 Wi‑Fi/有线后可断开再连接；开启「断线自动重连」可在隧道异常退出后重试。", err)

	case strings.Contains(lower, "endpoint"), strings.Contains(lower, "address is required"),
		strings.Contains(lower, "allowedips"), strings.Contains(lower, "validate"),
		strings.Contains(lower, "配置不完整"), strings.Contains(lower, "为必填项"):
		return fmt.Sprintf("配置不完整：%v\n\n请通过「快速向导」填写 Endpoint、服务端公钥与本地私钥，"+
			"或使用「剪贴板导入 / 粘贴配置 / 拖放 .conf / 导入二维码图片」。", err)

	case strings.Contains(lower, "二维码"), strings.Contains(lower, "qr"), strings.Contains(lower, "无法解析"),
		strings.Contains(lower, "粘贴内容"):
		return fmt.Sprintf("配置导入失败：%v\n\n请确认内容为完整 WireGuard 文本（含 [Interface]/[Peer]），"+
			"或清晰的二维码 PNG/JPEG。摄像头实时扫码为后续功能。", err)

	case strings.Contains(lower, "保存"), strings.Contains(lower, "write"), strings.Contains(lower, "permission"),
		strings.Contains(lower, "mkdir"):
		return fmt.Sprintf("写入失败：%v\n\n请检查 ~/.config/mywgclient/ 目录是否可写。", err)

	case strings.Contains(lower, "已取消"), strings.Contains(lower, "canceled"), strings.Contains(lower, "cancelled"):
		return fmt.Sprintf("操作已取消：%v", err)

	case strings.Contains(lower, "already"), strings.Contains(lower, "已处于"):
		return fmt.Sprintf("状态冲突：%v\n\n请先断开当前连接再试，避免重复连接导致状态不同步。", err)

	default:
		return fmt.Sprintf("出错：%v\n\n若问题与权限相关，请提权后重试；也可查看「详细日志」或「连接诊断」。", err)
	}
}

// Wrap annotates err; if err is nil returns nil.
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(Friendly(err))
}
