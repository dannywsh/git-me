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
)

// installUpdateMenu 将更新命令安装到 win 对应的原生菜单，并在用户修改更新偏好或语言时同步菜单。
// 输入参数 win 为主窗口，无返回值。
func (a *App) installUpdateMenu(win *mygo.Window) {
	if a.updates == nil {
		return
	}
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
	if a.updates.selfInstall {
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

// updateMenu 按 goos 和 locale 构建原生菜单，返回包含检查命令和更新偏好的菜单。
func (a *App) updateMenu(goos, locale string) *mygo.Menu {
	msg := locales.In(locale)
	check := &mygo.MenuItem{
		Label: msg.CheckForUpdates(),
		Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.updates.selfInstall {
				updater.CheckForUpdates()
			} else {
				a.updates.checkRelease(true)
			}
		},
	}
	preferences := &mygo.MenuItem{Label: msg.UpdateSettings(), Submenu: a.updatePreferences(msg)}
	if goos != "darwin" {
		return mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleHelp, Submenu: []*mygo.MenuItem{check, preferences}}})
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			check,
			preferences,
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
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
