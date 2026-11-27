package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/diag"
	"github.com/user/mywgclient/internal/errs"
	"github.com/user/mywgclient/internal/platform"
	"github.com/user/mywgclient/internal/tunnel"
)

func (ui *clientUI) refreshPrivilegeBanner() {
	ui.plat = platform.Snapshot()
	var parts []string
	if ui.plat.TipZH != "" {
		parts = append(parts, ui.plat.TipZH)
	}
	if ui.plat.OS == "windows" && !ui.plat.WintunOK {
		parts = append(parts, ui.plat.WintunDetail)
	}
	if len(parts) == 0 {
		ui.privBanner.SetText("权限检查通过。")
		return
	}
	ui.privBanner.SetText(strings.Join(parts, "\n"))
}

func (ui *clientUI) copySudoCommand() {
	cmd := ui.plat.SudoCommand
	if cmd == "" {
		cmd = platform.RebuildElevatedCommand()
	}
	ui.app.Clipboard().SetContent(cmd)
	ui.appendLog("info", "已复制提权命令: "+cmd)
	dialog.ShowInformation("提权命令", "已复制到剪贴板（含完整绝对路径）：\n\n"+cmd+"\n\n请在终端粘贴执行，或以管理员重新打开本程序。", ui.win)
}

func (ui *clientUI) copyVirtualIP() {
	ip := strings.TrimSpace(ui.virtIP.Text)
	if ip == "" || ip == "—" {
		ip = stripCIDR(ui.address.Text)
	}
	if ip == "" {
		dialog.ShowInformation("虚拟 IP", "当前没有可用的虚拟 IP。", ui.win)
		return
	}
	ui.app.Clipboard().SetContent(ip)
	ui.appendLog("info", "已复制虚拟 IP: "+ip)
	dialog.ShowInformation("虚拟 IP", "已复制: "+ip, ui.win)
}

func (ui *clientUI) showLogsDialog() {
	ui.refreshLogView()
	ui.mu.Lock()
	var all strings.Builder
	for _, l := range ui.logs {
		all.WriteString(l.Text)
	}
	text := all.String()
	ui.mu.Unlock()
	entry := widget.NewMultiLineEntry()
	entry.SetText(text)
	entry.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(entry)
	scroll.SetMinSize(fyne.NewSize(640, 400))
	copyBtn := widget.NewButton("复制全部日志", func() {
		ui.app.Clipboard().SetContent(text)
	})
	exportBtn := widget.NewButton("导出到文件…", func() {
		ui.exportLogs()
	})
	content := container.NewBorder(nil, container.NewHBox(copyBtn, exportBtn), nil, nil, scroll)
	d := dialog.NewCustom("详细日志", "关闭", content, ui.win)
	d.Resize(fyne.NewSize(680, 480))
	d.Show()
}

func (ui *clientUI) exportLogs() {
	ui.mu.Lock()
	var all strings.Builder
	for _, l := range ui.logs {
		all.WriteString(l.Text)
	}
	text := all.String()
	ui.mu.Unlock()
	dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		if uc == nil {
			return
		}
		defer uc.Close()
		path := uc.URI().Path()
		if path == "" {
			path = filepath.Join(os.TempDir(), "mywgclient-"+time.Now().Format("20060102-150405")+".log")
		}
		if !strings.HasSuffix(strings.ToLower(path), ".log") && !strings.HasSuffix(strings.ToLower(path), ".txt") {
			path += ".log"
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		ui.appendLog("info", "日志已导出到 "+path)
		dialog.ShowInformation("导出日志", "已保存到：\n"+path, ui.win)
	}, ui.win)
}

func (ui *clientUI) runDiagnostics() {
	cfg, _ := ui.formConfig()
	ui.appendLog("info", "正在运行连接诊断…")
	go func() {
		results := diag.RunWithOptions(cfg, diag.Options{Tunnel: ui.tun})
		summary := diag.Format(results)
		fyne.Do(func() {
			ui.appendLog("info", strings.TrimSpace(summary))
			entry := widget.NewMultiLineEntry()
			entry.SetText(summary)
			entry.Wrapping = fyne.TextWrapWord
			scroll := container.NewScroll(entry)
			scroll.SetMinSize(fyne.NewSize(560, 360))
			copyBtn := widget.NewButton("复制诊断结果", func() {
				ui.app.Clipboard().SetContent(summary)
			})
			content := container.NewBorder(nil, copyBtn, nil, nil, scroll)
			d := dialog.NewCustom("连接诊断", "关闭", content, ui.win)
			d.Resize(fyne.NewSize(600, 440))
			d.Show()
		})
	}()
}

func (ui *clientUI) testTUN() {
	ui.appendLog("info", "正在测试 TUN 设备创建…")
	go func() {
		name, err := tunnel.TestTUN(1420)
		fyne.Do(func() {
			if err != nil {
				msg := errs.Friendly(err)
				ui.appendLog("error", "TUN 测试失败: "+err.Error())
				dialog.ShowError(fmt.Errorf("%s", msg), ui.win)
				ui.refreshPrivilegeBanner()
				return
			}
			ui.appendLog("info", "TUN 测试成功，临时接口: "+name)
			dialog.ShowInformation("TUN 测试", "成功创建并销毁临时接口: "+name+"\n权限看起来正常。", ui.win)
		})
	}()
}

// testConfig verifies WireGuard handshake without installing full-tunnel routes.
func (ui *clientUI) testConfig() {
	st := ui.tun.Status()
	if st != tunnel.StatusDisconnected {
		dialog.ShowInformation("测试配置", "请先断开当前连接，再运行「测试配置」（测试不会安装 0.0.0.0/0 路由）。", ui.win)
		return
	}
	cfg, err := ui.formConfig()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		return
	}
	ui.userStop = false
	ui.setStatus(tunnel.StatusTesting)
	ui.appendLog("info", fmt.Sprintf("测试配置：拉起隧道并等待握手（最多 %s），不安装全局路由…", handshakeTestTimeout))
	go func() {
		err := ui.tun.TestHandshake(cfg, handshakeTestTimeout)
		fyne.Do(func() {
			ui.setStatus(tunnel.StatusDisconnected)
			if err != nil {
				ui.appendLog("error", "测试配置失败: "+err.Error())
				dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
				return
			}
			ui.appendLog("info", "测试配置成功：已完成握手，隧道已拆除，系统路由未改动。")
			dialog.ShowInformation("测试配置", "握手成功！\n密钥与 Endpoint 看起来正常。\n已拆除临时隧道，未安装默认路由。", ui.win)
		})
	}()
}

func (ui *clientUI) showGlobalRouteCommands() {
	iface := ui.tun.Name()
	if iface == "" {
		iface = "wg0"
	}
	cmds := tunnel.GlobalRouteCommands(iface)
	text := "全局模式一键路由命令（需管理员权限）：\n\n" + strings.Join(cmds, "\n") +
		"\n\n说明：客户端连接时会自动尝试安装半默认路由，断开时清理。"
	entry := widget.NewMultiLineEntry()
	entry.SetText(text)
	entry.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(entry)
	scroll.SetMinSize(fyne.NewSize(560, 220))
	copyBtn := widget.NewButton("复制命令", func() {
		ui.app.Clipboard().SetContent(strings.Join(cmds, "\n"))
	})
	content := container.NewBorder(nil, copyBtn, nil, nil, scroll)
	d := dialog.NewCustom("全局路由命令", "关闭", content, ui.win)
	d.Resize(fyne.NewSize(600, 300))
	d.Show()
}

func (ui *clientUI) toggleAutostart(on bool) {
	res := platform.SetAutostart(on)
	if !res.OK {
		ui.autostartChk.SetChecked(!on)
		ui.appendLog("warn", "自启动设置失败: "+res.TipZH)
		dialog.ShowError(fmt.Errorf("%s", res.TipZH), ui.win)
		return
	}
	if ui.settings == nil {
		ui.settings = config.DefaultSettings()
	}
	ui.settings.Autostart = res.Enabled
	_ = config.SaveSettings(ui.settings)
	ui.appendLog("info", res.TipZH)
	dialog.ShowInformation("开机自启", res.TipZH, ui.win)
}

func (ui *clientUI) importFromClipboard() {
	clip := strings.TrimSpace(ui.app.Clipboard().Content())
	if clip == "" {
		dialog.ShowInformation("剪贴板导入", "剪贴板为空。请先复制完整 WireGuard 配置或 vpn:// 文本。", ui.win)
		return
	}
	cfg, err := config.ParseImportText(clip)
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		ui.appendLog("error", "剪贴板导入失败: "+err.Error())
		return
	}
	ui.applyImported(cfg, "已从剪贴板一键导入配置")
	dialog.ShowInformation("导入成功", "已从剪贴板填充配置，可直接连接或先「测试配置」。", ui.win)
}
