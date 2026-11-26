package app

import (
	"runtime"

	"git-me/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

const (
	checkAtStartupID = "updates.automatic-checks"
	autoInstallID    = "updates.automatic-install"
	settingsMenuID   = "app.settings"
)

// installUpdateMenu 将应用设置与更新命令安装到 win 对应的原生菜单，并在用户修改更新偏好或语言时同步菜单。
// 输入参数 win 为主窗口，无返回值。
func (a *App) installUpdateMenu(win *mygo.Window) {
	var menu *mygo.Menu
	install := func(locale string) {
		menu = a.updateMenu(runtime.GOOS, locale)
		if runtime.GOOS == "darwin" {
			mygo.App.SetMenu(menu)
		} else {
			win.SetMenu(menu)
		}
	}
	install(mygo.App.Locale())
	win.OnClosed(mygo.App.OnLocaleChanged(install))
	if a.updates != nil && a.updates.selfInstall {
		win.OnClosed(updater.OnChange(func() {
			startup := menu.ItemByID(checkAtStartupID)
			autoInstall := menu.ItemByID(autoInstallID)
			automaticChecks := updater.AutomaticChecks()
			if startup.IsChecked() != automaticChecks {
				startup.SetChecked(automaticChecks)
			}
			if autoInstall.IsEnabled() != automaticChecks {
				autoInstall.SetEnabled(automaticChecks)
			}
			if on := updater.AutomaticDownloads(); autoInstall.IsChecked() != on {
				autoInstall.SetChecked(on)
			}
		}))
	}
}

// updateMenu 按 goos 和 locale 构建系统原生菜单，包含设置入口及更新命令。
// 输入参数 goos 为目标操作系统，locale 为界面语言标识；返回构建好的 *mygo.Menu。
func (a *App) updateMenu(goos, locale string) *mygo.Menu {
	msg := locales.In(locale)
	settings := &mygo.MenuItem{
		ID:          settingsMenuID,
		Label:       msg.Settings(),
		Accelerator: "CmdOrCtrl+,",
		Click: func(*mygo.MenuItem, *mygo.Window) {
			a.openSettingsWindow()
		},
	}

	var check, preferences *mygo.MenuItem
	if a.updates != nil {
		check = &mygo.MenuItem{
			Label: msg.CheckForUpdates(),
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.updates.selfInstall {
					updater.CheckForUpdates()
				} else {
					a.updates.checkRelease(true)
				}
			},
		}
		preferences = &mygo.MenuItem{Label: msg.UpdateSettings(), Submenu: a.updatePreferences(msg)}
	}

	if goos != "darwin" {
		openFolder := &mygo.MenuItem{
			Label:       msg.OpenFolder(),
			Accelerator: "CmdOrCtrl+O",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.PickRepoFolder()
			},
		}
		items := []*mygo.MenuItem{
			{Role: mygo.RoleFileMenu, Submenu: []*mygo.MenuItem{
				openFolder,
				mygo.Separator(),
				settings,
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			}},
		}
		if check != nil {
			items = append(items, &mygo.MenuItem{
				Label:   msg.HelpMenu(),
				Submenu: []*mygo.MenuItem{check, preferences},
			})
		}
		return mygo.NewMenu(items)
	}

	appItems := []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		mygo.Separator(),
		settings,
		mygo.Separator(),
	}
	if check != nil {
		appItems = append(appItems, check, preferences, mygo.Separator())
	}
	appItems = append(appItems,
		&mygo.MenuItem{Role: mygo.RoleServices},
		mygo.Separator(),
		&mygo.MenuItem{Role: mygo.RoleHide},
		&mygo.MenuItem{Role: mygo.RoleHideOthers},
		&mygo.MenuItem{Role: mygo.RoleUnhide},
		mygo.Separator(),
		&mygo.MenuItem{Role: mygo.RoleQuit},
	)
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: appItems},
		{Role: mygo.RoleFileMenu},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	})
}

// updatePreferences 根据当前安装方式和 msg 返回更新设置子菜单中的复选项。
func (a *App) updatePreferences(msg locales.Messages) []*mygo.MenuItem {
	startup := &mygo.MenuItem{
		ID: checkAtStartupID, Type: mygo.MenuItemCheckbox,
		Label: msg.CheckAtStartup(), Checked: a.Settings.CheckUpdatesOnLaunch, Disabled: mygo.IsDev(),
		Click: func(item *mygo.MenuItem, _ *mygo.Window) {
			a.Settings.CheckUpdatesOnLaunch = item.IsChecked()
			a.SaveSettings()
		},
	}
	if !a.updates.selfInstall {
		return []*mygo.MenuItem{startup}
	}
	startup.Label = msg.CheckAutomatically()
	startup.Checked = updater.AutomaticChecks()
	startup.Click = func(item *mygo.MenuItem, _ *mygo.Window) {
		updater.SetAutomaticChecks(item.IsChecked())
	}
	autoInstall := &mygo.MenuItem{
		ID: autoInstallID, Type: mygo.MenuItemCheckbox,
		Label: msg.InstallUpdatesAutomatically(), Checked: updater.AutomaticDownloads(),
		Disabled: !startup.Checked,
		Click: func(item *mygo.MenuItem, _ *mygo.Window) {
			updater.SetAutomaticDownloads(item.IsChecked())
		},
	}
	return []*mygo.MenuItem{startup, autoInstall}
}
