//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

const launchAgentLabel = "com.user.mywgclient"

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func setAutostart(enable bool) AutostartResult {
	path, err := launchAgentPath()
	if err != nil {
		return AutostartResult{OK: false, TipZH: "无法确定用户目录: " + err.Error()}
	}
	if !enable {
		_ = os.Remove(path)
		return AutostartResult{
			OK: true, Enabled: false, Path: path,
			TipZH: "已删除 LaunchAgent（" + path + "）。也可在「系统设置 → 通用 → 登录项」中移除。",
		}
	}
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.Abs(exe)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return AutostartResult{OK: false, TipZH: "无法创建 LaunchAgents 目录: " + err.Error()}
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>LimitLoadToSessionType</key><string>Aqua</string>
</dict>
</plist>
`, launchAgentLabel, exe)
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return AutostartResult{OK: false, TipZH: "写入 LaunchAgent 失败: " + err.Error()}
	}
	return AutostartResult{
		OK: true, Enabled: true, Path: path,
		TipZH: "已写入 LaunchAgent: " + path + "。macOS 也可能要求在「登录项」手动允许；创建 utun 通常仍需管理员权限（sudo）。",
	}
}

func autostartStatus() AutostartResult {
	path, err := launchAgentPath()
	if err != nil {
		return AutostartResult{OK: false, TipZH: err.Error()}
	}
	_, err = os.Stat(path)
	enabled := err == nil
	tip := "未启用 LaunchAgent 自启动"
	if enabled {
		tip = "已启用: " + path
	}
	return AutostartResult{OK: true, Enabled: enabled, Path: path, TipZH: tip}
}
