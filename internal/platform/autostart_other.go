//go:build !linux && !darwin && !windows

package platform

func setAutostart(enable bool) AutostartResult {
	_ = enable
	return AutostartResult{
		OK:    false,
		TipZH: "当前平台不支持自动开机启动，请手动将程序加入系统启动项。",
	}
}

func autostartStatus() AutostartResult {
	return AutostartResult{OK: true, Enabled: false, TipZH: "当前平台无自启动检测"}
}
