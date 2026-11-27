//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const runValueName = "mywgclient"

func setAutostart(enable bool) AutostartResult {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return AutostartResult{
			OK: false,
			TipZH: "无法打开注册表 Run 键（可能被策略限制）: " + err.Error() +
				"。可手动：Win+R → shell:startup，放入快捷方式。",
		}
	}
	defer key.Close()

	if !enable {
		_ = key.DeleteValue(runValueName)
		return AutostartResult{
			OK: true, Enabled: false,
			TipZH: "已从当前用户「Run」注册表移除自启动。若仍启动，请检查「任务管理器 → 启动应用」。",
		}
	}
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.Abs(exe)
	val := fmt.Sprintf(`"%s"`, exe)
	if err := key.SetStringValue(runValueName, val); err != nil {
		return AutostartResult{OK: false, TipZH: "写入注册表失败: " + err.Error()}
	}
	return AutostartResult{
		OK: true, Enabled: true, Path: val,
		TipZH: "已写入 HKCU\\…\\Run。注意：TUN 仍需管理员；普通登录自启动可能无权限创建 Wintun，届时请右键「以管理员身份运行」。",
	}
}

func autostartStatus() AutostartResult {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return AutostartResult{OK: false, TipZH: "无法读取注册表: " + err.Error()}
	}
	defer key.Close()
	val, _, err := key.GetStringValue(runValueName)
	if err != nil {
		return AutostartResult{OK: true, Enabled: false, TipZH: "未启用注册表自启动"}
	}
	return AutostartResult{OK: true, Enabled: true, Path: val, TipZH: "已启用: " + val}
}
