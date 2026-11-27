package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/crypto"
	"github.com/user/mywgclient/internal/errs"
)

func (ui *clientUI) shouldShowWizard() bool {
	if ui.settings == nil {
		return true
	}
	if ui.settings.WizardCompleted {
		return false
	}
	list, _, _ := config.ListProfiles()
	if len(list) > 0 {
		return false
	}
	path, err := config.DefaultProfilePath()
	if err != nil {
		return true
	}
	if _, err := os.Stat(path); err == nil {
		return false
	}
	return strings.TrimSpace(ui.privateKey.Text) == "" || strings.TrimSpace(ui.endpoint.Text) == ""
}

func (ui *clientUI) shouldShowOnboarding() bool {
	if ui.settings == nil {
		return true
	}
	return !ui.settings.OnboardingDone
}

func (ui *clientUI) showOnboarding() {
	body := widget.NewRichTextFromMarkdown("## 欢迎使用 Cown VPN\n\n" +
		"约 1 分钟完成首次配置：\n\n" +
		"1. **确认权限**：Linux/macOS 需 sudo；Windows 需管理员 + wintun.dll。点「测试 TUN」或「复制提权命令」。\n" +
		"2. **导入配置**（任选其一）：\n" +
		"   - 「快速向导」填写三项\n" +
		"   - 「剪贴板一键导入」\n" +
		"   - 「粘贴配置…」\n" +
		"   - 拖放 .conf 到窗口\n" +
		"   - 「导入二维码图片…」\n" +
		"3. 可选：**测试配置**（验证握手，不改系统路由）\n" +
		"4. 选择分流/全局 → **连接**\n" +
		"5. 可用「管理档案」切换/重命名/删除/设默认；托盘可快速连接/断开（退出会清理路由）\n\n" +
		"> 本客户端不会自动修改系统 DNS（split DNS 需系统侧配置）。\n")
	body.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(560, 360))

	d := dialog.NewCustomConfirm("首次使用说明", "开始快速向导", "我知道了", scroll, func(ok bool) {
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		ui.settings.OnboardingDone = true
		if err := config.SaveSettings(ui.settings); err != nil {
			ui.appendLog("warn", "保存引导状态失败: "+err.Error())
		}
		if ok {
			ui.showWizard()
		}
	}, ui.win)
	d.Resize(fyne.NewSize(620, 480))
	d.Show()
}

func (ui *clientUI) showWizard() {
	endpoint := widget.NewEntry()
	endpoint.SetPlaceHolder("vpn.example.com:51820")
	endpoint.SetText(ui.endpoint.Text)

	serverPub := widget.NewEntry()
	serverPub.SetPlaceHolder("服务器 PublicKey（base64）")
	serverPub.SetText(ui.publicKey.Text)

	clientPriv := widget.NewPasswordEntry()
	clientPriv.SetPlaceHolder("本机 PrivateKey（base64）")
	clientPriv.SetText(ui.privateKey.Text)

	derived := widget.NewLabel("公钥将自动派生")
	derived.Wrapping = fyne.TextWrapBreak
	clientPriv.OnChanged = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			derived.SetText("公钥将自动派生")
			return
		}
		k, err := crypto.ParseKey(s)
		if err != nil {
			derived.SetText("私钥格式无效")
			return
		}
		pub, err := crypto.PublicFromPrivate(k)
		if err != nil {
			derived.SetText("派生失败")
			return
		}
		derived.SetText("派生公钥: " + crypto.KeyToBase64(pub) + "\n（请把此公钥配置到服务器 [Peer]）")
	}
	if clientPriv.Text != "" {
		clientPriv.OnChanged(clientPriv.Text)
	}

	genBtn := widget.NewButton("生成本机密钥对", func() {
		kp, err := crypto.GenerateKeyPair()
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		clientPriv.SetText(crypto.KeyToBase64(kp.Private))
		clientPriv.OnChanged(clientPriv.Text)
	})

	hint := widget.NewLabel("只需填写三项：服务器地址、服务器公钥、本机私钥。\n" +
		"默认 Address=10.8.0.2/24，AllowedIPs=10.8.0.0/24，Keepalive=25。\n" +
		"也可关闭向导，改用剪贴板导入 / 拖放 .conf。")
	hint.Wrapping = fyne.TextWrapWord

	form := container.NewVBox(
		hint,
		widget.NewForm(
			widget.NewFormItem("服务器 Endpoint", endpoint),
			widget.NewFormItem("服务器公钥", serverPub),
			widget.NewFormItem("本机私钥", clientPriv),
			widget.NewFormItem("派生公钥", derived),
		),
		genBtn,
	)

	d := dialog.NewCustomConfirm("快速设置向导", "完成并填充", "稍后", form, func(ok bool) {
		if !ok {
			return
		}
		ep := strings.TrimSpace(endpoint.Text)
		spk := strings.TrimSpace(serverPub.Text)
		cpk := strings.TrimSpace(clientPriv.Text)
		if ep == "" || spk == "" || cpk == "" {
			dialog.ShowError(fmt.Errorf("请填写 Endpoint、服务器公钥与本机私钥"), ui.win)
			return
		}
		cfg := config.QuickSetup(ep, spk, cpk)
		if err := cfg.Validate(); err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		if _, err := crypto.ParseKey(cpk); err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		if _, err := crypto.ParseKey(spk); err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		ui.applyConfig(cfg)
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		ui.settings.WizardCompleted = true
		ui.settings.OnboardingDone = true
		_ = config.SaveSettings(ui.settings)
		id, err := config.SaveNamedProfile("", "向导配置", cfg)
		if err != nil {
			ui.appendLog("warn", "保存档案失败: "+err.Error())
		} else {
			ui.activeProfile = id
			_ = config.SetDefaultProfile(id)
			ui.refreshProfileSelect()
		}
		ui.appendLog("info", "向导完成：已填充默认 Address / AllowedIPs，并保存档案。")
	}, ui.win)
	d.Resize(fyne.NewSize(560, 460))
	d.Show()
}

func (ui *clientUI) pasteConfDialog() {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("在此粘贴完整 WireGuard 配置，或 vpn:// / wireguard:// 文本…")
	entry.SetMinRowsVisible(14)
	entry.Wrapping = fyne.TextWrapWord

	if clip := strings.TrimSpace(ui.app.Clipboard().Content()); clip != "" {
		lower := strings.ToLower(clip)
		if strings.Contains(lower, "[interface]") || strings.HasPrefix(lower, "vpn://") ||
			strings.HasPrefix(lower, "wireguard://") || strings.HasPrefix(lower, "wg://") {
			entry.SetText(clip)
		}
	}

	hint := widget.NewLabel("支持：标准 .conf 文本、vpn:// 前缀、Base64 包装的配置。也可直接点主界面「剪贴板一键导入」。")
	hint.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(hint, nil, nil, nil, entry)

	d := dialog.NewCustomConfirm("粘贴配置", "导入", "取消", content, func(ok bool) {
		if !ok {
			return
		}
		cfg, err := config.ParseImportText(entry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			ui.appendLog("error", "粘贴导入失败: "+err.Error())
			return
		}
		ui.applyImported(cfg, "已从粘贴文本导入配置")
		dialog.ShowInformation("导入成功", "配置已填充到表单，可直接连接。", ui.win)
	}, ui.win)
	d.Resize(fyne.NewSize(640, 480))
	d.Show()
}

func (ui *clientUI) importQRImage() {
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
		cfg, err := config.LoadQRImage(path)
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s\n\n提示：请导出/截取清晰的 WireGuard 配置二维码（PNG/JPEG）。", errs.Friendly(err)), ui.win)
			ui.appendLog("error", "二维码导入失败: "+err.Error())
			return
		}
		ui.applyImported(cfg, "已从二维码图片导入 "+filepath.Base(path))
		dialog.ShowInformation("导入成功", "已从二维码识别并填充配置。", ui.win)
	}, ui.win)
}
