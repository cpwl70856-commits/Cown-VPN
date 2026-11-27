//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopName = "mywgclient.desktop"

func autostartDesktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "autostart", desktopName), nil
}

func setAutostart(enable bool) AutostartResult {
	path, err := autostartDesktopPath()
	if err != nil {
		return AutostartResult{OK: false, TipZH: "无法确定用户目录: " + err.Error()}
	}
	if !enable {
		_ = os.Remove(path)
		return AutostartResult{
			OK: true, Enabled: false, Path: path,
			TipZH: "已移除 XDG 自启动项（" + path + "）。若桌面环境仍启动，请检查「启动应用程序」设置。",
		}
	}
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.Abs(exe)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return AutostartResult{OK: false, TipZH: "无法创建 autostart 目录: " + err.Error()}
	}
	body := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=Cown VPN
Comment=WireGuard desktop client
Exec=%s
Terminal=false
Categories=Network;
X-GNOME-Autostart-enabled=true
`, quoteDesktopExec(exe))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return AutostartResult{OK: false, TipZH: "写入失败: " + err.Error()}
	}
	return AutostartResult{
		OK: true, Enabled: true, Path: path,
		TipZH: "已写入 XDG 自启动: " + path + "。部分发行版仍需在「启动应用程序」中确认。创建 TUN 仍需 root（可用 pkexec/sudo 包装）。",
	}
}

func autostartStatus() AutostartResult {
	path, err := autostartDesktopPath()
	if err != nil {
		return AutostartResult{OK: false, TipZH: err.Error()}
	}
	_, err = os.Stat(path)
	enabled := err == nil
	tip := "未启用 XDG 自启动"
	if enabled {
		tip = "已启用: " + path
	}
	return AutostartResult{OK: true, Enabled: enabled, Path: path, TipZH: tip}
}

func quoteDesktopExec(s string) string {
	if filepath.Base(s) == s {
		return s
	}
	return `"` + s + `"`
}
