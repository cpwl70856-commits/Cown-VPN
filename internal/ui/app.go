// Package ui provides the Fyne desktop interface for mywgclient.
package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/crypto"
	"github.com/user/mywgclient/internal/errs"
	"github.com/user/mywgclient/internal/platform"
	"github.com/user/mywgclient/internal/tunnel"
)

const connectTimeout = 20 * time.Second
const handshakeTestTimeout = 15 * time.Second

// Run launches the mywgclient window and blocks until quit.
func Run() {
	tunnel.GuardPanic(func() {
		a := app.NewWithID("com.user.mywgclient")
		a.SetIcon(theme.ComputerIcon())
		applySystemTheme(a)

		w := a.NewWindow("Cown VPN — WireGuard 客户端")
		w.Resize(fyne.NewSize(880, 820))

		tun := tunnel.New()
		ui := newClientUI(a, w, tun)
		w.SetContent(ui.build())
		ui.setupTray()
		ui.setupDragDrop()
		ui.appendLog("info", "就绪。可向导配置、剪贴板一键导入、拖放 .conf，或粘贴配置文本。")
		ui.refreshPrivilegeBanner()
		_ = config.EnsureMigratedProfile()
		ui.loadInitialProfile()

		w.SetCloseIntercept(func() {
			if ui.settings != nil && ui.settings.MinimizeToTray {
				w.Hide()
				ui.appendLog("info", "已最小化到系统托盘（托盘菜单可退出并清理路由）。")
				return
			}
			ui.quitApp()
		})

		w.Show()
		if ui.shouldShowOnboarding() {
			ui.showOnboarding()
		} else if ui.shouldShowWizard() {
			ui.showWizard()
		}
		a.Run()
		tunnel.ForceCleanup("app-exit")
	})
}

type clientUI struct {
	app    fyne.App
	win    fyne.Window
	tun    *tunnel.Tunnel
	mu     sync.Mutex
	logBuf strings.Builder
	logs   []logLine

	settings *config.Settings
	plat     platform.Info

	statusLabel  *widget.Label
	statusDot    *widget.Label
	progress     *widget.ProgressBarInfinite
	virtIP       *widget.Label
	rxLabel      *widget.Label
	txLabel      *widget.Label
	rateLabel    *widget.Label
	handshakeLbl *widget.Label
	connectedLbl *widget.Label
	logEntry     *widget.Entry
	logFilter    *widget.Select
	privBanner   *widget.Label
	routeSelect  *widget.Select
	pubDerived   *widget.Label
	autoReconnect *widget.Check
	autostartChk  *widget.Check
	profileSelect *widget.Select
	profileIDs    []string
	activeProfile string

	privateKey *widget.Entry
	address    *widget.Entry
	endpoint   *widget.Entry
	publicKey  *widget.Entry
	allowedIPs *widget.Entry
	psk        *widget.Entry
	keepalive  *widget.Entry
	listenPort *widget.Entry
	mtu        *widget.Entry

	connectBtn *widget.Button
	statsStop  chan struct{}
	watchStop  chan struct{}
	userStop   bool
	connecting sync.Mutex // prevent double-connect races from tray+button

	trayStatus     *fyne.MenuItem
	trayConnect    *fyne.MenuItem
	trayDisconnect *fyne.MenuItem
}

type logLine struct {
	Level string
	Text  string
}

func newClientUI(a fyne.App, w fyne.Window, t *tunnel.Tunnel) *clientUI {
	s, err := config.LoadSettings()
	if err != nil {
		s = config.DefaultSettings()
	}
	ui := &clientUI{app: a, win: w, tun: t, settings: s, plat: platform.Snapshot()}
	ui.statusDot = widget.NewLabel("○")
	ui.statusLabel = widget.NewLabel("未连接")
	ui.progress = widget.NewProgressBarInfinite()
	ui.progress.Hide()
	ui.virtIP = widget.NewLabel("—")
	ui.rxLabel = widget.NewLabel("↓ 0 B")
	ui.txLabel = widget.NewLabel("↑ 0 B")
	ui.rateLabel = widget.NewLabel("速率 —")
	ui.handshakeLbl = widget.NewLabel("最近握手: —")
	ui.connectedLbl = widget.NewLabel("已连接时长: —")
	ui.logEntry = widget.NewMultiLineEntry()
	ui.logEntry.SetMinRowsVisible(6)
	ui.logEntry.Wrapping = fyne.TextWrapWord
	ui.logEntry.Disable()
	ui.privBanner = widget.NewLabel("")
	ui.privBanner.Wrapping = fyne.TextWrapWord
	ui.pubDerived = widget.NewLabel("本地公钥: （输入私钥后自动派生）")
	ui.pubDerived.Wrapping = fyne.TextWrapBreak

	ui.privateKey = widget.NewPasswordEntry()
	ui.privateKey.SetPlaceHolder("本机私钥 PrivateKey（base64）")
	ui.privateKey.OnChanged = func(s string) {
		ui.updateDerivedPublic(s)
	}
	ui.address = widget.NewEntry()
	ui.address.SetPlaceHolder("10.8.0.2/24")
	ui.address.SetText("10.8.0.2/24")
	ui.endpoint = widget.NewEntry()
	ui.endpoint.SetPlaceHolder("vpn.example.com:51820")
	ui.publicKey = widget.NewEntry()
	ui.publicKey.SetPlaceHolder("服务器公钥 PublicKey（base64）")
	ui.allowedIPs = widget.NewEntry()
	ui.allowedIPs.SetText("10.8.0.0/24")
	ui.psk = widget.NewPasswordEntry()
	ui.psk.SetPlaceHolder("可选预共享密钥 PresharedKey")
	ui.keepalive = widget.NewEntry()
	ui.keepalive.SetText("25")
	ui.listenPort = widget.NewEntry()
	ui.listenPort.SetPlaceHolder("0 = 系统分配")
	ui.mtu = widget.NewEntry()
	ui.mtu.SetText("1420")

	ui.autoReconnect = widget.NewCheck("断线自动重连", func(on bool) {
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		ui.settings.AutoReconnect = on
		_ = config.SaveSettings(ui.settings)
		if on {
			ui.appendLog("info", "已开启自动重连：隧道异常断开后将尝试重新连接。")
		} else {
			ui.appendLog("info", "已关闭自动重连。")
		}
	})
	if s != nil && s.AutoReconnect {
		ui.autoReconnect.SetChecked(true)
	}

	ui.autostartChk = widget.NewCheck("开机自启（尽力）", func(on bool) {
		ui.toggleAutostart(on)
	})
	if st := platform.AutostartStatus(); st.Enabled {
		ui.autostartChk.SetChecked(true)
	}

	level := "全部"
	if s != nil {
		switch s.LogLevel {
		case "info":
			level = "信息+"
		case "warn":
			level = "警告+"
		case "error":
			level = "仅错误"
		}
	}
	ui.logFilter = widget.NewSelect([]string{"全部", "信息+", "警告+", "仅错误"}, func(v string) {
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		switch v {
		case "信息+":
			ui.settings.LogLevel = "info"
		case "警告+":
			ui.settings.LogLevel = "warn"
		case "仅错误":
			ui.settings.LogLevel = "error"
		default:
			ui.settings.LogLevel = "all"
		}
		_ = config.SaveSettings(ui.settings)
		ui.refreshLogView()
	})
	ui.logFilter.SetSelected(level)

	mode := "分流模式 (split)"
	if s != nil && s.RouteMode == string(tunnel.RouteModeGlobal) {
		mode = "全局模式 (global)"
	}
	ui.routeSelect = widget.NewSelect([]string{"分流模式 (split)", "全局模式 (global)"}, func(v string) {
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		if strings.Contains(v, "global") {
			ui.settings.RouteMode = string(tunnel.RouteModeGlobal)
			ui.allowedIPs.SetText("0.0.0.0/0")
			ui.appendLog("info", "已切换为全局模式：连接时安装 0.0.0.0/1 与 128.0.0.0/1；断开必清理。")
		} else {
			ui.settings.RouteMode = string(tunnel.RouteModeSplit)
			if strings.TrimSpace(ui.allowedIPs.Text) == "0.0.0.0/0" {
				ui.allowedIPs.SetText("10.8.0.0/24")
			}
			ui.appendLog("info", "已切换为分流模式：仅路由 AllowedIPs 中的网段。")
		}
		_ = config.SaveSettings(ui.settings)
	})
	ui.routeSelect.SetSelected(mode)

	ui.profileSelect = widget.NewSelect([]string{}, func(name string) {
		ui.onProfileSelected(name)
	})
	ui.refreshProfileSelect()

	ui.connectBtn = widget.NewButton("连接", ui.toggleConnect)
	ui.connectBtn.Importance = widget.HighImportance
	ui.setStatus(tunnel.StatusDisconnected)
	return ui
}

func (ui *clientUI) build() fyne.CanvasObject {
	header := container.NewVBox(
		container.NewHBox(
			ui.statusDot,
			ui.statusLabel,
			widget.NewLabel("  |  虚拟 IP:"),
			ui.virtIP,
			widget.NewLabel("  |  "),
			ui.rxLabel,
			widget.NewLabel("  "),
			ui.txLabel,
			widget.NewLabel("  |  "),
			ui.rateLabel,
		),
		container.NewHBox(ui.handshakeLbl, widget.NewLabel("  |  "), ui.connectedLbl),
		ui.progress,
		ui.privBanner,
	)

	form := widget.NewForm(
		widget.NewFormItem("配置档案", ui.profileSelect),
		widget.NewFormItem("私钥 PrivateKey", ui.privateKey),
		widget.NewFormItem("派生公钥", ui.pubDerived),
		widget.NewFormItem("地址 Address", ui.address),
		widget.NewFormItem("服务器 Endpoint", ui.endpoint),
		widget.NewFormItem("服务器公钥", ui.publicKey),
		widget.NewFormItem("AllowedIPs", ui.allowedIPs),
		widget.NewFormItem("路由模式", ui.routeSelect),
		widget.NewFormItem("自动重连", ui.autoReconnect),
		widget.NewFormItem("开机自启", ui.autostartChk),
		widget.NewFormItem("PresharedKey", ui.psk),
		widget.NewFormItem("Keepalive (秒)", ui.keepalive),
		widget.NewFormItem("Listen Port", ui.listenPort),
		widget.NewFormItem("MTU", ui.mtu),
	)

	wizardBtn := widget.NewButton("快速向导", ui.showWizard)
	clipBtn := widget.NewButton("剪贴板一键导入", ui.importFromClipboard)
	pasteBtn := widget.NewButton("粘贴配置…", ui.pasteConfDialog)
	qrBtn := widget.NewButton("导入二维码图片…", ui.importQRImage)
	importBtn := widget.NewButton("导入 .conf…", ui.importConf)
	exportBtn := widget.NewButton("导出 .conf…", ui.exportConf)

	profilesBtn := widget.NewButton("管理档案…", ui.showProfileManager)
	copyIPBtn := widget.NewButton("复制虚拟 IP", ui.copyVirtualIP)
	logsBtn := widget.NewButton("详细日志", ui.showLogsDialog)
	diagBtn := widget.NewButton("连接诊断", ui.runDiagnostics)
	testCfgBtn := widget.NewButton("测试配置", ui.testConfig)
	testTUNBtn := widget.NewButton("测试 TUN", ui.testTUN)
	sudoBtn := widget.NewButton("复制提权命令", ui.copySudoCommand)
	saveBtn := widget.NewButton("保存到档案", ui.saveCurrentToProfile)
	loadBtn := widget.NewButton("加载默认档案", ui.loadInitialProfile)
	routeCmdBtn := widget.NewButton("全局路由命令", ui.showGlobalRouteCommands)
	onboardBtn := widget.NewButton("使用说明", ui.showOnboarding)
	exportLogBtn := widget.NewButton("导出日志", ui.exportLogs)
	clearLog := widget.NewButton("清空日志", func() {
		ui.mu.Lock()
		ui.logBuf.Reset()
		ui.logs = nil
		ui.mu.Unlock()
		ui.logEntry.SetText("")
	})

	row1 := container.NewHBox(ui.connectBtn, wizardBtn, clipBtn, pasteBtn, qrBtn, importBtn, exportBtn)
	row2 := container.NewHBox(diagBtn, testCfgBtn, testTUNBtn, sudoBtn, routeCmdBtn, copyIPBtn, logsBtn)
	row3 := container.NewHBox(profilesBtn, saveBtn, loadBtn, onboardBtn, exportLogBtn, clearLog,
		widget.NewLabel("日志级别"), ui.logFilter)

	logBox := container.NewBorder(
		widget.NewLabelWithStyle("日志", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil, ui.logEntry,
	)

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Cown VPN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			header,
			widget.NewSeparator(),
			row1,
			row2,
			row3,
		),
		nil, nil, nil,
		container.NewBorder(form, nil, nil, nil, logBox),
	)
}

func (ui *clientUI) setupDragDrop() {
	ui.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			path := u.Path()
			if path == "" {
				continue
			}
			lower := strings.ToLower(path)
			if strings.HasSuffix(lower, ".conf") || strings.HasSuffix(lower, ".txt") {
				cfg, err := config.LoadFile(path)
				if err != nil {
					dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
					ui.appendLog("error", "拖放导入失败: "+err.Error())
					return
				}
				ui.applyImported(cfg, "已拖放导入 "+filepath.Base(path))
				dialog.ShowInformation("导入成功", "已从拖放文件填充配置。", ui.win)
				return
			}
			if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
				strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".gif") {
				cfg, err := config.LoadQRImage(path)
				if err != nil {
					dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
					return
				}
				ui.applyImported(cfg, "已从拖放二维码导入 "+filepath.Base(path))
				return
			}
		}
		dialog.ShowInformation("拖放", "请拖放 .conf / .txt 配置文件，或二维码图片。", ui.win)
	})
}

func (ui *clientUI) updateDerivedPublic(priv string) {
	priv = strings.TrimSpace(priv)
	if priv == "" {
		ui.pubDerived.SetText("本地公钥: （输入私钥后自动派生）")
		return
	}
	k, err := crypto.ParseKey(priv)
	if err != nil {
		ui.pubDerived.SetText("本地公钥: 私钥格式无效")
		return
	}
	pub, err := crypto.PublicFromPrivate(k)
	if err != nil {
		ui.pubDerived.SetText("本地公钥: 派生失败")
		return
	}
	ui.pubDerived.SetText("本地公钥: " + crypto.KeyToBase64(pub))
}

func (ui *clientUI) routeMode() tunnel.RouteMode {
	if ui.settings != nil && ui.settings.RouteMode == string(tunnel.RouteModeGlobal) {
		return tunnel.RouteModeGlobal
	}
	return tunnel.RouteModeSplit
}

func (ui *clientUI) setStatus(s tunnel.Status) {
	switch s {
	case tunnel.StatusConnected:
		ui.statusDot.SetText("●")
		ui.statusLabel.SetText("已连接")
		ui.connectBtn.SetText("断开")
		ui.connectBtn.Importance = widget.DangerImportance
		ui.connectBtn.Enable()
		ui.connectBtn.Refresh()
		ui.progress.Stop()
		ui.progress.Hide()
		ui.setFormEnabled(false)
		ui.updateTrayMenu(tunnel.StatusConnected)
	case tunnel.StatusConnecting, tunnel.StatusTesting:
		ui.statusDot.SetText("◐")
		if s == tunnel.StatusTesting {
			ui.statusLabel.SetText("测试配置中…")
		} else {
			ui.statusLabel.SetText("正在连接…")
		}
		ui.connectBtn.SetText("取消")
		ui.connectBtn.Importance = widget.MediumImportance
		ui.connectBtn.Refresh()
		ui.progress.Show()
		ui.progress.Start()
		ui.setFormEnabled(false)
		ui.updateTrayMenu(s)
	default:
		ui.statusDot.SetText("○")
		ui.statusLabel.SetText("未连接")
		ui.connectBtn.SetText("连接")
		ui.connectBtn.Importance = widget.HighImportance
		ui.connectBtn.Enable()
		ui.connectBtn.Refresh()
		ui.progress.Stop()
		ui.progress.Hide()
		ui.setFormEnabled(true)
		ui.virtIP.SetText("—")
		ui.rxLabel.SetText("↓ 0 B")
		ui.txLabel.SetText("↑ 0 B")
		ui.rateLabel.SetText("速率 —")
		ui.handshakeLbl.SetText("最近握手: —")
		ui.connectedLbl.SetText("已连接时长: —")
		ui.updateTrayMenu(tunnel.StatusDisconnected)
	}
}

func (ui *clientUI) setFormEnabled(on bool) {
	entries := []*widget.Entry{
		ui.privateKey, ui.address, ui.endpoint, ui.publicKey,
		ui.allowedIPs, ui.psk, ui.keepalive, ui.listenPort, ui.mtu,
	}
	for _, e := range entries {
		if on {
			e.Enable()
		} else {
			e.Disable()
		}
	}
	if on {
		ui.routeSelect.Enable()
		ui.autoReconnect.Enable()
		ui.autostartChk.Enable()
		ui.profileSelect.Enable()
	} else {
		ui.routeSelect.Disable()
		ui.autoReconnect.Disable()
		ui.autostartChk.Disable()
		ui.profileSelect.Disable()
	}
}

func (ui *clientUI) appendLog(level, msg string) {
	if level == "" {
		level = "info"
	}
	line := fmt.Sprintf("%s  [%s] %s\n", time.Now().Format("15:04:05"), strings.ToUpper(level), msg)
	ui.mu.Lock()
	ui.logs = append(ui.logs, logLine{Level: level, Text: line})
	ui.logBuf.WriteString(line)
	ui.mu.Unlock()
	ui.refreshLogView()
}

func (ui *clientUI) refreshLogView() {
	min := "all"
	if ui.settings != nil && ui.settings.LogLevel != "" {
		min = ui.settings.LogLevel
	}
	ui.mu.Lock()
	var b strings.Builder
	for _, l := range ui.logs {
		if !logLevelVisible(l.Level, min) {
			continue
		}
		b.WriteString(l.Text)
	}
	text := b.String()
	ui.mu.Unlock()
	ui.logEntry.SetText(text)
	ui.logEntry.CursorRow = strings.Count(text, "\n")
}

func logLevelVisible(level, min string) bool {
	rank := func(l string) int {
		switch strings.ToLower(l) {
		case "error":
			return 3
		case "warn", "warning":
			return 2
		case "info":
			return 1
		default:
			return 0
		}
	}
	switch min {
	case "error":
		return rank(level) >= 3
	case "warn":
		return rank(level) >= 2
	case "info":
		return rank(level) >= 1
	default:
		return true
	}
}

func (ui *clientUI) formConfig() (*config.Config, error) {
	ka, _ := strconv.Atoi(strings.TrimSpace(ui.keepalive.Text))
	lp, _ := strconv.Atoi(strings.TrimSpace(ui.listenPort.Text))
	mtu, _ := strconv.Atoi(strings.TrimSpace(ui.mtu.Text))
	cfg := config.FromForm(
		ui.privateKey.Text,
		ui.address.Text,
		ui.endpoint.Text,
		ui.publicKey.Text,
		ui.allowedIPs.Text,
		ui.psk.Text,
		ka, lp, mtu,
	)
	if ui.activeProfile != "" {
		if _, meta, err := config.LoadNamedProfile(ui.activeProfile); err == nil && meta != nil {
			cfg.Name = meta.Name
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if peer := cfg.PrimaryPeer(); peer != nil && peer.Endpoint == "" {
		return nil, fmt.Errorf("Endpoint 为必填项（例如 vpn.example.com:51820）")
	}
	return cfg, nil
}

func (ui *clientUI) applyConfig(cfg *config.Config) {
	ui.privateKey.SetText(cfg.PrivateKey)
	ui.updateDerivedPublic(cfg.PrivateKey)
	ui.address.SetText(cfg.Address)
	if cfg.ListenPort > 0 {
		ui.listenPort.SetText(strconv.Itoa(cfg.ListenPort))
	} else {
		ui.listenPort.SetText("")
	}
	if cfg.MTU > 0 {
		ui.mtu.SetText(strconv.Itoa(cfg.MTU))
	}
	if p := cfg.PrimaryPeer(); p != nil {
		ui.publicKey.SetText(p.PublicKey)
		ui.endpoint.SetText(p.Endpoint)
		ui.allowedIPs.SetText(strings.Join(p.AllowedIPs, ", "))
		ui.psk.SetText(p.PresharedKey)
		if p.PersistentKeepalive > 0 {
			ui.keepalive.SetText(strconv.Itoa(p.PersistentKeepalive))
		}
		if tunnelHasFull(p.AllowedIPs) {
			ui.routeSelect.SetSelected("全局模式 (global)")
		} else if ui.settings == nil || ui.settings.RouteMode != string(tunnel.RouteModeGlobal) {
			ui.routeSelect.SetSelected("分流模式 (split)")
		}
	}
}

func tunnelHasFull(allowed []string) bool {
	for _, a := range allowed {
		if strings.TrimSpace(a) == "0.0.0.0/0" {
			return true
		}
	}
	return false
}

func (ui *clientUI) toggleConnect() {
	st := ui.tun.Status()
	if st == tunnel.StatusConnected || st == tunnel.StatusConnecting || st == tunnel.StatusTesting {
		ui.userStop = true
		ui.stopWatchers()
		_ = ui.tun.Disconnect()
		ui.setStatus(tunnel.StatusDisconnected)
		ui.appendLog("info", "已断开，并清理本会话添加的路由。")
		return
	}

	if !ui.connecting.TryLock() {
		ui.appendLog("warn", "连接操作进行中，忽略重复点击。")
		return
	}
	defer ui.connecting.Unlock()

	cfg, err := ui.formConfig()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		ui.appendLog("error", "校验失败: "+err.Error())
		return
	}
	ui.userStop = false
	ui.beginConnect(cfg, false)
}

func (ui *clientUI) beginConnect(cfg *config.Config, isReconnect bool) {
	if ui.tun.Status() != tunnel.StatusDisconnected {
		ui.appendLog("warn", "已有连接/连接中，跳过重复连接。")
		return
	}
	mode := ui.routeMode()
	ui.setStatus(tunnel.StatusConnecting)
	if isReconnect {
		ui.appendLog("info", fmt.Sprintf("自动重连 %s（%s）…", cfg.PrimaryPeer().Endpoint, modeLabel(mode)))
	} else {
		ui.appendLog("info", fmt.Sprintf("正在连接 %s（%s）…", cfg.PrimaryPeer().Endpoint, modeLabel(mode)))
	}

	go func() {
		err := ui.tun.ConnectWithOptions(cfg, connectTimeout, tunnel.ConnectOptions{RouteMode: mode})
		fyne.Do(func() {
			if err != nil {
				if ui.tun.Status() == tunnel.StatusDisconnected && ui.userStop {
					return
				}
				ui.setStatus(tunnel.StatusDisconnected)
				friendly := errs.Friendly(err)
				ui.appendLog("error", "连接失败: "+err.Error())
				if !isReconnect {
					dialog.ShowError(fmt.Errorf("%s", friendly), ui.win)
				} else {
					ui.appendLog("warn", "自动重连失败，将在隧道再次断开或手动连接时重试。")
				}
				return
			}
			ui.setStatus(tunnel.StatusConnected)
			ui.virtIP.SetText(stripCIDR(ui.tun.Address()))
			ui.appendLog("info", fmt.Sprintf("已连接：%s（%s）", ui.tun.Name(), ui.tun.Address()))
			if re := ui.tun.LastRouteError(); re != nil {
				ui.appendLog("warn", "路由警告: "+tunnel.FormatRouteError(re))
			}
			if mode == tunnel.RouteModeGlobal {
				ui.appendLog("info", "全局模式：已尝试安装 0.0.0.0/1 与 128.0.0.0/1。若失败请点「全局路由命令」。断开时会清理。")
			}
			if err := config.RememberSuccess(cfg, string(mode)); err != nil {
				ui.appendLog("warn", "记住成功配置失败: "+err.Error())
			}
			if s, err := config.LoadSettings(); err == nil {
				ui.settings = s
				ui.autoReconnect.SetChecked(s.AutoReconnect)
			}
			ui.startStats()
			ui.watchTunnel(cfg)
		})
	}()
}

func modeLabel(m tunnel.RouteMode) string {
	if m == tunnel.RouteModeGlobal {
		return "全局"
	}
	return "分流"
}

func stripCIDR(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

func (ui *clientUI) startStats() {
	ui.stopStats()
	ui.statsStop = make(chan struct{})
	stop := ui.statsStop
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				st, err := ui.tun.StatsWithRate()
				connectedAt := ui.tun.ConnectedAt()
				if err != nil {
					continue
				}
				fyne.Do(func() {
					ui.rxLabel.SetText("↓ " + formatBytes(st.RxBytes))
					ui.txLabel.SetText("↑ " + formatBytes(st.TxBytes))
					ui.rateLabel.SetText(fmt.Sprintf("↓%s/s ↑%s/s", formatRate(st.RxPerSec), formatRate(st.TxPerSec)))
					ui.handshakeLbl.SetText("最近握手: " + formatHandshake(st.LastHandshakeUnix))
					ui.connectedLbl.SetText("已连接时长: " + formatDurationSince(connectedAt))
					if ui.trayStatus != nil {
						ui.trayStatus.Label = trayStatusText(true, st)
						ui.refreshTrayMenu()
					}
				})
			}
		}
	}()
}

func trayStatusText(connected bool, st tunnel.RateStats) string {
	if !connected {
		return "状态: 未连接"
	}
	hs := "无握手"
	if st.LastHandshakeUnix > 0 {
		hs = "已握手"
	}
	return fmt.Sprintf("状态: 已连接 · %s · ↓%s", hs, formatBytes(st.RxBytes))
}

func (ui *clientUI) stopStats() {
	if ui.statsStop != nil {
		close(ui.statsStop)
		ui.statsStop = nil
	}
}

func (ui *clientUI) stopWatchers() {
	ui.stopStats()
	if ui.watchStop != nil {
		close(ui.watchStop)
		ui.watchStop = nil
	}
}

func (ui *clientUI) watchTunnel(cfg *config.Config) {
	if ui.watchStop != nil {
		close(ui.watchStop)
		ui.watchStop = nil
	}
	wait := ui.tun.Wait()
	if wait == nil {
		return
	}
	ui.watchStop = make(chan struct{})
	stop := ui.watchStop
	go func() {
		select {
		case <-stop:
			return
		case <-wait:
			fyne.Do(func() {
				if ui.userStop {
					return
				}
				ui.stopStats()
				// Ensure routes cleaned even if device died unexpectedly.
				_ = ui.tun.Disconnect()
				ui.setStatus(tunnel.StatusDisconnected)
				ui.appendLog("warn", "隧道意外断开（可能网络切换）。已清理路由。")
				if ui.settings != nil && ui.settings.AutoReconnect && !ui.userStop {
					ui.appendLog("info", "将在 3 秒后自动重连…")
					go func() {
						time.Sleep(3 * time.Second)
						fyne.Do(func() {
							if ui.userStop || ui.tun.Status() != tunnel.StatusDisconnected {
								return
							}
							if !ui.autoReconnect.Checked {
								return
							}
							ui.beginConnect(cfg, true)
						})
					}()
				}
			})
		}
	}()
}

func (ui *clientUI) importConf() {
	dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		if rc == nil {
			return
		}
		defer rc.Close()
		path := rc.URI().Path()
		cfg, err := config.LoadFile(path)
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			ui.appendLog("error", "导入失败: "+err.Error())
			return
		}
		ui.applyImported(cfg, "已导入 "+filepath.Base(path))
	}, ui.win)
}

func (ui *clientUI) applyImported(cfg *config.Config, logMsg string) {
	ui.applyConfig(cfg)
	ui.appendLog("info", logMsg)
	if ui.settings == nil {
		ui.settings = config.DefaultSettings()
	}
	ui.settings.WizardCompleted = true
	ui.settings.OnboardingDone = true
	_ = config.SaveSettings(ui.settings)
	name := cfg.Name
	if name == "" || name == "imported" || name == "profile" {
		name = "导入配置"
	}
	id, err := config.SaveNamedProfile(ui.activeProfile, name, cfg)
	if err != nil {
		ui.appendLog("warn", "自动保存档案失败: "+err.Error())
	} else {
		ui.activeProfile = id
		ui.refreshProfileSelect()
	}
}

func (ui *clientUI) exportConf() {
	cfg, err := ui.formConfig()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		return
	}
	dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		if uc == nil {
			return
		}
		defer uc.Close()
		path := uc.URI().Path()
		if !strings.HasSuffix(strings.ToLower(path), ".conf") {
			path += ".conf"
		}
		if err := config.SaveFile(path, cfg); err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			ui.appendLog("error", "导出失败: "+err.Error())
			return
		}
		ui.appendLog("info", "已导出到 "+path)
	}, ui.win)
}

func (ui *clientUI) quitApp() {
	ui.userStop = true
	ui.stopWatchers()
	_ = ui.tun.Disconnect()
	tunnel.ForceCleanup("quit")
	ui.app.Quit()
}

func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.2f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatRate(bps float64) string {
	if bps < 0 {
		bps = 0
	}
	return formatBytes(int64(bps))
}

func formatHandshake(unixSec int64) string {
	if unixSec <= 0 {
		return "尚无（等待握手）"
	}
	t := time.Unix(unixSec, 0)
	ago := time.Since(t).Round(time.Second)
	if ago < 0 {
		ago = 0
	}
	return fmt.Sprintf("%s（%s 前）", t.Format("15:04:05"), ago)
}

func formatDurationSince(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t).Round(time.Second)
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分%d秒", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d分%d秒", m, s)
	}
	return fmt.Sprintf("%d秒", s)
}
