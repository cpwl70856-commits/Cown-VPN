// Package platform provides OS privilege, Wintun, autostart, and restart helpers for mywgclient.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Info summarizes the current runtime environment for the UI.
type Info struct {
	OS           string
	Elevated     bool
	WintunOK     bool
	WintunDetail string
	SudoCommand  string
	TipZH        string
}

// Snapshot gathers privilege / driver tips for the current process.
func Snapshot() Info {
	info := Info{
		OS:          runtime.GOOS,
		Elevated:    IsElevated(),
		SudoCommand: RebuildElevatedCommand(),
	}
	switch runtime.GOOS {
	case "windows":
		ok, detail := DetectWintun()
		info.WintunOK = ok
		info.WintunDetail = detail
		if !info.Elevated {
			info.TipZH = "当前未以管理员身份运行。创建 TUN / 修改路由需要「以管理员身份运行」。请右键可执行文件 → 以管理员身份运行，或复制下方 PowerShell 提权命令。"
		} else if !ok {
			info.TipZH = "未检测到可用的 Wintun。请从 https://www.wintun.net 下载对应架构的 wintun.dll，放到本程序同目录（或 amd64/arm64 子目录），也可安装官方 WireGuard for Windows。"
		}
	case "darwin":
		if !info.Elevated {
			info.TipZH = "当前不是 root。创建 utun 与配置路由通常需要提权。请点击「复制提权命令」后在「终端」粘贴执行（命令含完整绝对路径），或以 sudo 重新启动。"
		}
	case "linux":
		if !info.Elevated {
			info.TipZH = "当前不是 root。创建 TUN 与 ip route 通常需要提权。请点击「复制提权命令」后在终端执行（含完整路径与参数），或使用 sudo/pkexec 启动。"
		}
	}
	return info
}

// RebuildElevatedCommand returns a shell/cmd line to relaunch this process with elevation.
// Always uses the absolute executable path so copy-paste works from any cwd.
func RebuildElevatedCommand() string {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.Abs(exe)
	args := append([]string{quoteShell(exe)}, quoteArgs(os.Args[1:])...)
	joined := strings.Join(args, " ")
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf(`powershell -Command "Start-Process -FilePath '%s' -Verb RunAs"`, strings.ReplaceAll(exe, "'", "''"))
	case "darwin", "linux":
		return "sudo " + joined
	default:
		return joined
	}
}

func quoteArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, quoteShell(a))
	}
	return out
}

func quoteShell(s string) string {
	if s == "" {
		return `""`
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '"' || r == '\'' || r == '\\'
	}) < 0 {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
