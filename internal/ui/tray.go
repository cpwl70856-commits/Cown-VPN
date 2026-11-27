package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	"github.com/user/mywgclient/internal/tunnel"
)

func (ui *clientUI) setupTray() {
	desk, ok := ui.app.(desktop.App)
	if !ok {
		ui.appendLog("warn", "当前平台不支持系统托盘，关闭窗口将直接退出。")
		if ui.settings != nil {
			ui.settings.MinimizeToTray = false
		}
		return
	}

	ui.trayStatus = fyne.NewMenuItem("状态: 未连接", nil)
	ui.trayStatus.Disabled = true

	ui.trayConnect = fyne.NewMenuItem("连接", func() {
		fyne.Do(func() {
			ui.win.Show()
			ui.win.RequestFocus()
			if ui.tun.Status() == tunnel.StatusDisconnected {
				ui.toggleConnect()
			}
		})
	})
	ui.trayDisconnect = fyne.NewMenuItem("断开", func() {
		fyne.Do(func() {
			st := ui.tun.Status()
			if st == tunnel.StatusConnected || st == tunnel.StatusConnecting || st == tunnel.StatusTesting {
				ui.toggleConnect()
			}
		})
	})
	ui.refreshTrayMenu()
	desk.SetSystemTrayIcon(theme.ComputerIcon())
	ui.updateTrayMenu(tunnel.StatusDisconnected)
	ui.appendLog("info", "系统托盘已启用：关闭窗口将最小化到托盘；退出会清理路由。")
}

func (ui *clientUI) refreshTrayMenu() {
	desk, ok := ui.app.(desktop.App)
	if !ok || ui.trayConnect == nil || ui.trayDisconnect == nil {
		return
	}
	showItem := fyne.NewMenuItem("显示主窗口", func() {
		fyne.Do(func() {
			ui.win.Show()
			ui.win.RequestFocus()
		})
	})
	quitItem := fyne.NewMenuItem("退出（清理路由）", func() {
		fyne.Do(func() {
			ui.quitApp()
		})
	})
	items := []*fyne.MenuItem{showItem, fyne.NewMenuItemSeparator()}
	if ui.trayStatus != nil {
		items = append(items, ui.trayStatus, fyne.NewMenuItemSeparator())
	}
	items = append(items, ui.trayConnect, ui.trayDisconnect, fyne.NewMenuItemSeparator(), quitItem)
	menu := fyne.NewMenu("Cown VPN", items...)
	desk.SetSystemTrayMenu(menu)
}

func (ui *clientUI) updateTrayMenu(st tunnel.Status) {
	if ui.trayConnect == nil || ui.trayDisconnect == nil {
		return
	}
	connected := st == tunnel.StatusConnected
	busy := st == tunnel.StatusConnecting || st == tunnel.StatusTesting
	ui.trayConnect.Disabled = connected || busy
	ui.trayDisconnect.Disabled = !connected && !busy
	if ui.trayStatus != nil {
		switch st {
		case tunnel.StatusConnected:
			ui.trayStatus.Label = "状态: 已连接"
		case tunnel.StatusConnecting:
			ui.trayStatus.Label = "状态: 正在连接…"
		case tunnel.StatusTesting:
			ui.trayStatus.Label = "状态: 测试配置中…"
		default:
			ui.trayStatus.Label = "状态: 未连接"
		}
	}
	// Re-apply menu so Disabled flags refresh on all platforms.
	ui.refreshTrayMenu()
}
