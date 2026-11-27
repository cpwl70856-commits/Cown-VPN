package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/user/mywgclient/internal/config"
	"github.com/user/mywgclient/internal/errs"
)

func (ui *clientUI) refreshProfileSelect() {
	list, defID, err := config.ListProfiles()
	if err != nil {
		ui.appendLog("warn", "读取档案列表失败: "+err.Error())
		return
	}
	names := make([]string, 0, len(list)+1)
	ids := make([]string, 0, len(list)+1)
	names = append(names, "（当前表单）")
	ids = append(ids, "")
	selected := "（当前表单）"
	for _, p := range list {
		label := p.Name
		if p.ID == defID {
			label += " ★"
		}
		names = append(names, label)
		ids = append(ids, p.ID)
		if ui.activeProfile != "" && p.ID == ui.activeProfile {
			selected = label
		} else if ui.activeProfile == "" && p.ID == defID {
			selected = label
			ui.activeProfile = defID
		}
	}
	ui.profileIDs = ids
	ui.profileSelect.Options = names
	ui.profileSelect.SetSelected(selected)
	ui.profileSelect.Refresh()
}

func (ui *clientUI) onProfileSelected(name string) {
	if name == "" || name == "（当前表单）" {
		return
	}
	// Map display name back to id.
	idx := -1
	for i, opt := range ui.profileSelect.Options {
		if opt == name {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(ui.profileIDs) {
		return
	}
	id := ui.profileIDs[idx]
	if id == "" || id == ui.activeProfile {
		return
	}
	cfg, meta, err := config.LoadNamedProfile(id)
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		return
	}
	ui.activeProfile = id
	ui.applyConfig(cfg)
	ui.appendLog("info", "已切换档案: "+meta.Name)
}

func (ui *clientUI) loadInitialProfile() {
	_ = config.EnsureMigratedProfile()
	ui.refreshProfileSelect()
	if ui.settings != nil && ui.settings.DefaultProfileID != "" {
		cfg, meta, err := config.LoadNamedProfile(ui.settings.DefaultProfileID)
		if err == nil {
			ui.activeProfile = meta.ID
			ui.applyConfig(cfg)
			ui.refreshProfileSelect()
			ui.appendLog("info", "已加载默认档案: "+meta.Name)
			return
		}
	}
	list, defID, _ := config.ListProfiles()
	if defID != "" {
		cfg, meta, err := config.LoadNamedProfile(defID)
		if err == nil {
			ui.activeProfile = meta.ID
			ui.applyConfig(cfg)
			ui.refreshProfileSelect()
			ui.appendLog("info", "已加载默认档案: "+meta.Name)
			return
		}
	}
	if len(list) > 0 {
		cfg, meta, err := config.LoadNamedProfile(list[0].ID)
		if err == nil {
			ui.activeProfile = meta.ID
			ui.applyConfig(cfg)
			ui.refreshProfileSelect()
			ui.appendLog("info", "已加载档案: "+meta.Name)
			return
		}
	}
	// Fall back to legacy profile.conf
	path, err := config.DefaultProfilePath()
	if err != nil {
		return
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		ui.appendLog("info", "尚无已保存配置（可通过向导或导入创建）")
		return
	}
	ui.applyConfig(cfg)
	ui.appendLog("info", "已加载 "+path)
}

func (ui *clientUI) saveCurrentToProfile() {
	cfg, err := ui.formConfig()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
		return
	}
	nameEntry := widget.NewEntry()
	nameEntry.SetText(cfg.Name)
	if nameEntry.Text == "" || nameEntry.Text == "profile" {
		nameEntry.SetText("我的配置")
	}
	form := widget.NewForm(widget.NewFormItem("档案名称", nameEntry))
	dialog.ShowCustomConfirm("保存到档案", "保存", "取消", form, func(ok bool) {
		if !ok {
			return
		}
		name := strings.TrimSpace(nameEntry.Text)
		id, err := config.SaveNamedProfile(ui.activeProfile, name, cfg)
		if err != nil {
			dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
			return
		}
		ui.activeProfile = id
		if ui.settings == nil {
			ui.settings = config.DefaultSettings()
		}
		ui.settings.RouteMode = string(ui.routeMode())
		ui.settings.AutoReconnect = ui.autoReconnect.Checked
		_ = config.SaveSettings(ui.settings)
		ui.refreshProfileSelect()
		ui.appendLog("info", "已保存档案: "+name)
		dialog.ShowInformation("保存成功", "档案「"+name+"」已保存。", ui.win)
	}, ui.win)
}

func (ui *clientUI) showProfileManager() {
	list, defID, err := config.ListProfiles()
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	if len(list) == 0 {
		dialog.ShowInformation("配置档案", "还没有档案。请先「保存到档案」或导入配置。", ui.win)
		return
	}

	options := make([]string, 0, len(list))
	idByLabel := map[string]string{}
	for _, p := range list {
		label := p.Name
		if p.ID == defID {
			label += " ★默认"
		}
		options = append(options, label)
		idByLabel[label] = p.ID
	}
	sel := widget.NewSelect(options, nil)
	sel.SetSelected(options[0])

	renameBtn := widget.NewButton("重命名", func() {
		id := idByLabel[sel.Selected]
		if id == "" {
			return
		}
		entry := widget.NewEntry()
		entry.SetText(strings.TrimSuffix(strings.TrimSuffix(sel.Selected, " ★默认"), " ★"))
		dialog.ShowCustomConfirm("重命名", "确定", "取消", entry, func(ok bool) {
			if !ok {
				return
			}
			if err := config.RenameProfile(id, entry.Text); err != nil {
				dialog.ShowError(err, ui.win)
				return
			}
			ui.appendLog("info", "已重命名档案")
			ui.refreshProfileSelect()
			ui.showProfileManager()
		}, ui.win)
	})
	defaultBtn := widget.NewButton("设为默认", func() {
		id := idByLabel[sel.Selected]
		if id == "" {
			return
		}
		if err := config.SetDefaultProfile(id); err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		ui.activeProfile = id
		ui.appendLog("info", "已设为默认档案")
		ui.refreshProfileSelect()
		ui.showProfileManager()
	})
	deleteBtn := widget.NewButton("删除", func() {
		id := idByLabel[sel.Selected]
		if id == "" {
			return
		}
		dialog.ShowConfirm("删除档案", "确定删除选中档案？此操作不可恢复。", func(ok bool) {
			if !ok {
				return
			}
			if err := config.DeleteProfile(id); err != nil {
				dialog.ShowError(err, ui.win)
				return
			}
			if ui.activeProfile == id {
				ui.activeProfile = ""
			}
			ui.appendLog("info", "已删除档案")
			ui.refreshProfileSelect()
		}, ui.win)
	})
	exportBtn := widget.NewButton("导出 .conf…", func() {
		id := idByLabel[sel.Selected]
		if id == "" {
			return
		}
		dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			path := uc.URI().Path()
			if !strings.HasSuffix(strings.ToLower(path), ".conf") {
				path += ".conf"
			}
			if err := config.ExportProfile(id, path); err != nil {
				dialog.ShowError(fmt.Errorf("%s", errs.Friendly(err)), ui.win)
				return
			}
			ui.appendLog("info", "档案已导出到 "+path)
		}, ui.win)
	})
	switchBtn := widget.NewButton("切换到此档案", func() {
		id := idByLabel[sel.Selected]
		if id == "" {
			return
		}
		cfg, meta, err := config.LoadNamedProfile(id)
		if err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		ui.activeProfile = id
		ui.applyConfig(cfg)
		ui.refreshProfileSelect()
		ui.appendLog("info", "已切换档案: "+meta.Name)
	})

	body := container.NewVBox(
		widget.NewLabel("选择档案："),
		sel,
		container.NewHBox(switchBtn, defaultBtn, renameBtn, deleteBtn, exportBtn),
	)
	d := dialog.NewCustom("管理配置档案", "关闭", body, ui.win)
	d.Resize(fyne.NewSize(520, 220))
	d.Show()
}
