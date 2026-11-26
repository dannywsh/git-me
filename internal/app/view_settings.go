package app

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"git-me/internal/git"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

const (
	settingsSectionGit     = "git"
	settingsSectionGeneral = "general"

	settingsTitleBarHeight = 52
	settingsSidebarWidth   = 200
)

// settingsSVG 解析用于设置窗口左侧 Sidebar 的 16x16 线性图标。
// 输入参数 shapes 为 SVG 内部图形元素字符串；返回解析后的 *ui.SVG。
func settingsSVG(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	settingsGitIcon     = settingsSVG(`<circle cx="4.5" cy="3.5" r="1.5"/><circle cx="4.5" cy="12.5" r="1.5"/><circle cx="11.5" cy="5.5" r="1.5"/><path d="M4.5 5v6M11.5 7c0 2.5-2 3.5-7 3.5"/>`)
	settingsGeneralIcon = settingsSVG(`<path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h11"/><circle cx="5.5" cy="4.5" r="1.3" fill="currentColor"/><circle cx="10.5" cy="8" r="1.3" fill="currentColor"/><circle cx="6.5" cy="11.5" r="1.3" fill="currentColor"/>`)
)

// settingsState 维护独立原生“设置”窗口的分类选择、Git 路径表单及异步探测状态。
type settingsState struct {
	section         string
	gitPathInput    string
	detectedPath    string
	detectedVersion string
	detectErr       string
	checking        bool
	saving          bool
	errorMsg        string
	statusMsg       string
	checkSeq        atomic.Uint64
	saveSeq         atomic.Uint64
}

// runSettingsUI 在 UI 主线程执行设置状态变更，并在设置窗口存在时请求重绘。
// 输入参数 fn 为需要在主线程运行的更新函数；无返回值。
func (a *App) runSettingsUI(fn func()) {
	a.RunUI(func() {
		fn()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}

// invalidateMainWindow 请求重绘主窗口，使设置变更立即反映到主界面。
// 无输入参数，无返回值。
func (a *App) invalidateMainWindow() {
	a.uiMu.Lock()
	win := a.Win
	a.uiMu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}

// openSettingsWindow 打开或聚焦独立的桌面原生“设置”窗口，并异步检测当前生效的 Git 可执行文件。
// 无输入参数，无返回值。
func (a *App) openSettingsWindow() {
	if a.settingsState.section == "" {
		a.settingsState.section = settingsSectionGit
	}
	a.settingsState.gitPathInput = a.Settings.GitPath
	a.settingsState.errorMsg = ""
	a.settingsState.statusMsg = ""
	a.settingsState.saving = false
	a.probeGitPathAsync(a.Settings.GitPath)

	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.settingsWin.Invalidate()
		return
	}

	a.uiMu.Lock()
	parent := a.Win
	a.uiMu.Unlock()

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:          "设置",
		Parent:         parent,
		Width:          740,
		Height:         480,
		MinWidth:       620,
		MinHeight:      400,
		StateKey:       "git-me-settings",
		TitleBarStyle:  mygo.TitleBarHiddenInset,
		TitleBarHeight: settingsTitleBarHeight,
		Vibrancy:       mygo.VibrancySidebar,
		Content:        ui.View(a.viewSettingsWindow),
	})
	a.settingsWin = win
	win.OnClosed(func() {
		a.RunUI(func() {
			if a.settingsWin == win {
				a.settingsWin = nil
			}
		})
	})
}

// closeSettingsWindow 关闭当前打开的原生“设置”窗口（若存在）。
// 无输入参数，无返回值。
func (a *App) closeSettingsWindow() {
	if a.settingsWin != nil {
		a.settingsWin.Close()
	}
}

// probeGitPathAsync 在后台校验指定 Git 路径（空字符串表示自动检测）并刷新设置窗口的状态展示。
// 输入参数 path 为待校验的 Git 可执行文件路径；无返回值。
func (a *App) probeGitPathAsync(path string) {
	seq := a.settingsState.checkSeq.Add(1)
	a.settingsState.checking = true
	a.settingsState.detectErr = ""

	go func() {
		resolved, version, err := git.ValidateGitExecutable(path)
		a.runSettingsUI(func() {
			if a.settingsState.checkSeq.Load() != seq {
				return
			}
			a.settingsState.checking = false
			if err != nil {
				a.settingsState.detectedPath = ""
				a.settingsState.detectedVersion = ""
				a.settingsState.detectErr = err.Error()
				return
			}
			a.settingsState.detectedPath = resolved
			a.settingsState.detectedVersion = version
			a.settingsState.detectErr = ""
		})
	}()
}

// pickGitExecutable 弹出系统原生文件选择对话框，供用户手动选择本地 Git 可执行文件（如 Windows 下的 git.exe）。
// 无输入参数，无返回值。
func (a *App) pickGitExecutable() {
	parent := a.settingsWin
	if parent == nil {
		a.uiMu.Lock()
		parent = a.Win
		a.uiMu.Unlock()
	}
	defaultDir := ""
	for _, candidate := range []string{strings.TrimSpace(a.settingsState.gitPathInput), a.settingsState.detectedPath} {
		if candidate != "" {
			defaultDir = filepath.Dir(candidate)
			break
		}
	}
	opts := mygo.OpenDialogOptions{
		Parent:          parent,
		Title:           "选择 Git 可执行文件",
		DefaultPath:     defaultDir,
		ShowHiddenFiles: true,
	}
	if runtime.GOOS == "windows" {
		opts.Filters = []mygo.FileFilter{
			{Name: "Git 可执行文件 (*.exe)", Extensions: []string{"exe"}},
			{Name: "所有文件 (*.*)", Extensions: []string{"*"}},
		}
	}

	go func() {
		paths, err := mygo.Dialog.Open(opts)
		if err != nil || len(paths) == 0 {
			return
		}
		selected := paths[0]
		a.runSettingsUI(func() {
			a.settingsState.gitPathInput = selected
			a.settingsState.errorMsg = ""
			a.settingsState.statusMsg = ""
			a.probeGitPathAsync(selected)
		})
	}()
}

// resetGitPath 清空自定义 Git 路径并立即保存，恢复自动检测。
// 无输入参数，无返回值。
func (a *App) resetGitPath() {
	a.settingsState.gitPathInput = ""
	a.settingsState.errorMsg = ""
	a.settingsState.statusMsg = ""
	a.saveGitSettings()
}

// saveGitSettings 在后台校验并保存用户配置的 Git 路径，立即生效并刷新所有已打开仓库。
// 无输入参数，无返回值。
func (a *App) saveGitSettings() {
	if a.settingsState.saving {
		return
	}
	rawPath := strings.TrimSpace(a.settingsState.gitPathInput)
	seq := a.settingsState.saveSeq.Add(1)
	a.settingsState.saving = true
	a.settingsState.errorMsg = ""
	a.settingsState.statusMsg = ""

	go func() {
		var savedPath, detectedPath, version, detectErr string
		if rawPath != "" {
			resolved, ver, err := git.ValidateGitExecutable(rawPath)
			if err != nil {
				a.runSettingsUI(func() {
					if a.settingsState.saveSeq.Load() != seq {
						return
					}
					a.settingsState.saving = false
					a.settingsState.errorMsg = err.Error()
				})
				return
			}
			savedPath = resolved
			detectedPath = resolved
			version = ver
		} else {
			resolved, ver, err := git.ValidateGitExecutable("")
			if err != nil {
				detectErr = err.Error()
			} else {
				detectedPath = resolved
				version = ver
			}
		}

		a.runSettingsUI(func() {
			if a.settingsState.saveSeq.Load() != seq {
				return
			}
			a.settingsState.saving = false
			prevPath := a.Settings.GitPath
			a.Settings.GitPath = savedPath
			git.SetGitPath(savedPath)
			if err := a.SaveSettings(); err != nil {
				a.Settings.GitPath = prevPath
				git.SetGitPath(prevPath)
				a.settingsState.errorMsg = "无法保存设置：" + err.Error()
				return
			}
			a.settingsState.gitPathInput = savedPath
			a.settingsState.detectedPath = detectedPath
			a.settingsState.detectedVersion = version
			a.settingsState.detectErr = detectErr
			if savedPath == "" {
				a.settingsState.statusMsg = "已恢复自动检测 Git"
			} else {
				a.settingsState.statusMsg = "已保存 Git 路径设置"
			}
			if detectErr != "" {
				a.GlobalError = detectErr
			} else {
				a.GlobalError = ""
			}
			for _, tab := range a.Tabs {
				tab.RefreshAsync(false)
			}
		})
	}()
}

// viewSettingsWindow 使用 MyGo 官方原生 Sidebar、Vibrancy 与 Stretch 双栏范式渲染独立的桌面“设置”窗口。
// 输入参数 c 为设置窗口的 *ui.Context；无返回值。
func (a *App) viewSettingsWindow(c *ui.Context) {
	a.flushPendingUI()
	if a.settingsState.section == "" {
		a.settingsState.section = settingsSectionGit
	}

	c.OnWindowShortcut(ui.Cmd, 'w', a.closeSettingsWindow)
	c.OnWindowShortcut(0, ui.KeyEscape, a.closeSettingsWindow)

	t := c.Theme()
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		side := ui.Column(c).Width(settingsSidebarWidth).Shrink(0)
		if !vibrant {
			side.Background(t.Surface)
		}
		side.Children(func() {
			ui.Box(c).Height(settingsTitleBarHeight).Shrink(0).DragWindow()
			list := ui.Sidebar(c.Key("settings-sidebar"), &a.settingsState.section, func() {
				ui.SidebarItem(c, settingsSectionGit, settingsGitIcon, "Git")
				ui.SidebarItem(c, settingsSectionGeneral, settingsGeneralIcon, "通用")
			}).Grow(1).Label("程序设置")
			if vibrant {
				list.Background(ui.Transparent)
			}
		})

		ui.Box(c).Width(1).Background(t.Border)

		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			ui.Row(c).Height(settingsTitleBarHeight).Shrink(0).AlignItems(ui.Center).
				Padding(0, bar.Right+24, 0, 24).DragWindow().Children(func() {
				ui.Text(c, settingsSectionTitle(a.settingsState.section)).FontSize(15).Bold().SingleLine()
			})
			ui.Divider(c)
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(20, 24).Gap(18).MaxWidth(540).Children(func() {
					switch a.settingsState.section {
					case settingsSectionGeneral:
						a.viewSettingsGeneralSection(c)
					default:
						a.viewSettingsGitSection(c)
					}
				})
			})
		})
	})
}

// settingsSectionTitle 返回设置侧边栏分类 ID 对应的右侧标题栏名称。
// 输入参数 section 为分类标识，返回中文标题文本。
func settingsSectionTitle(section string) string {
	if section == settingsSectionGeneral {
		return "通用"
	}
	return "Git"
}

// viewSettingsGroup 渲染原生分组设置卡片：上方为分组小标题，下方为圆角 Surface 卡片容器。
// 输入参数 c 为 *ui.Context，title 为分组标题，content 为卡片内部构建回调；无返回值。
func viewSettingsGroup(c *ui.Context, title string, content func()) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted).Padding(0, 4)
		}
		ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Children(content)
	})
}

// viewSettingsSwitchRow 渲染一条标准桌面设置开关行：左侧标题与说明，右侧原生 ui.Switch 开关。
// 输入参数 c 为 *ui.Context，key 为控件唯一键，title 为设置名，desc 为说明，value 为绑定布尔指针，onChange 为切换回调；无返回值。
func viewSettingsSwitchRow(c *ui.Context, key, title, desc string, value *bool, onChange func()) {
	t := c.Theme()
	ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500)
			if desc != "" {
				ui.Text(c, desc).FontSize(11).TextColor(t.TextMuted).Wrap()
			}
		})
		if ui.Switch(c.Key(key), value).Label(title).Changed() && onChange != nil {
			onChange()
		}
	})
}

// viewSettingsGitSection 渲染“Git”分类设置页，提供手动路径选择、即时版本检测与保存。
// 输入参数 c 为 *ui.Context；无返回值。
func (a *App) viewSettingsGitSection(c *ui.Context) {
	t := c.Theme()
	viewSettingsGroup(c, "Git 命令行程序", func() {
		ui.Column(c).Padding(14, 16).Gap(10).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "可执行文件路径").FontSize(13).FontWeight(500)
				ui.Text(c, gitPathHint()).FontSize(11).TextColor(t.TextMuted).Wrap()
			})

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				inp := ui.TextInput(c.Key("settings-git-path"), &a.settingsState.gitPathInput).
					Placeholder(gitPathPlaceholder()).
					Invalid(a.settingsState.errorMsg != "").
					Grow(1).
					Shrink(1).
					MinWidth(0)
				if inp.Changed() {
					a.settingsState.errorMsg = ""
					a.settingsState.statusMsg = ""
				}
				if inp.Submitted() {
					a.saveGitSettings()
				}
				if ui.Button(c.Key("settings-browse-git"), "浏览…").Clicked() {
					a.pickGitExecutable()
				}
				if ui.Button(c.Key("settings-probe-git"), "检测").Disabled(a.settingsState.checking).Clicked() {
					a.probeGitPathAsync(a.settingsState.gitPathInput)
				}
			})

			if a.settingsState.errorMsg != "" {
				ui.Text(c, a.settingsState.errorMsg).FontSize(12).TextColor(t.Danger).Wrap()
			}
		})

		ui.Divider(c)
		a.viewGitStatusSection(c)
		ui.Divider(c)

		ui.Row(c).Padding(10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.settingsState.statusMsg != "" {
				ui.Text(c, a.settingsState.statusMsg).FontSize(12).TextColor(t.Success)
			}
			ui.Spacer(c)
			if ui.Button(c.Key("settings-reset-git"), "恢复自动检测").Disabled(a.settingsState.saving).Clicked() {
				a.resetGitPath()
			}
			if ui.PrimaryButton(c.Key("settings-save-git"), "保存").Disabled(a.settingsState.saving).Clicked() {
				a.saveGitSettings()
			}
		})
	})
}

// viewGitStatusSection 在 Git 设置卡片中展示当前 Git 可执行文件的检测结果或错误原因。
// 输入参数 c 为 *ui.Context；无返回值。
func (a *App) viewGitStatusSection(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(12, 16).Gap(4).Children(func() {
		switch {
		case a.settingsState.checking:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在检测 Git 可执行文件…").FontSize(12).TextColor(t.TextMuted)
			})
		case a.settingsState.detectErr != "":
			ui.Text(c, a.settingsState.detectErr).FontSize(12).TextColor(t.Danger).Wrap()
		case a.settingsState.detectedPath != "":
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "当前状态").FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, a.settingsState.detectedVersion).FontSize(12).Bold().TextColor(t.Success)
			})
			ui.Text(c, a.settingsState.detectedPath).Font("monospace").FontSize(11).TextColor(t.TextMuted).Selectable().Wrap()
		default:
			ui.Text(c, "点击「检测」验证当前 Git 配置是否可用").FontSize(12).TextColor(t.TextMuted)
		}
	})
}

// viewSettingsGeneralSection 渲染“通用”分类设置页，以原生开关行管理更新检查与界面偏好。
// 输入参数 c 为 *ui.Context；无返回值。
func (a *App) viewSettingsGeneralSection(c *ui.Context) {
	viewSettingsGroup(c, "软件更新", func() {
		if a.updates != nil && a.updates.selfInstall {
			autoChecks := updater.AutomaticChecks()
			viewSettingsSwitchRow(
				c,
				"settings-auto-checks",
				"自动检查更新",
				"后台定期检查 Gitme 是否有新版本发布",
				&autoChecks,
				func() { updater.SetAutomaticChecks(autoChecks) },
			)
		} else {
			viewSettingsSwitchRow(
				c,
				"settings-check-launch",
				"启动时自动检查更新",
				"启动 Gitme 时在后台检查 GitHub Releases 新版本",
				&a.Settings.CheckUpdatesOnLaunch,
				func() { _ = a.SaveSettings() },
			)
		}
	})

	viewSettingsGroup(c, "界面偏好", func() {
		viewSettingsSwitchRow(
			c,
			"settings-tree-mode",
			"目录树模式",
			"在左侧边栏按目录层级折叠展示变更文件与项目文件",
			&a.Settings.TreeMode,
			func() {
				_ = a.SaveSettings()
				a.invalidateMainWindow()
			},
		)
		ui.Divider(c)
		viewSettingsSwitchRow(
			c,
			"settings-unified-diff",
			"上下合并差异视图",
			"开启时以上下合并方式显示代码对比，关闭时采用左右分栏",
			&a.Settings.UnifiedDiff,
			func() {
				a.ApplyDiffLayoutToAllTabs()
				a.invalidateMainWindow()
			},
		)
		ui.Divider(c)
		viewSettingsSwitchRow(
			c,
			"settings-all-changes",
			"全部变更一页流",
			"在右侧主视图连续滚动浏览当前所有变更文件的代码差异",
			&a.Settings.AllChanges,
			func() {
				_ = a.SaveSettings()
				a.invalidateMainWindow()
			},
		)
	})
}

// gitPathHint 返回面向当前操作系统的 Git 路径配置说明文案。
// 无输入参数，返回中文提示文本。
func gitPathHint() string {
	if runtime.GOOS == "windows" {
		return `留空则自动从系统 PATH 或常见安装目录查找；若 Git 未写入环境变量或注册表，请手动选择 git.exe（例如 C:\Program Files\Git\cmd\git.exe）。`
	}
	return "留空则自动从系统 PATH 查找；如使用自定义或便携版 Git，可在此手动选择 git 可执行文件（例如 /usr/bin/git）。"
}

// gitPathPlaceholder 返回 Git 路径输入框的平台示例占位文本。
// 无输入参数，返回示例路径字符串。
func gitPathPlaceholder() string {
	if runtime.GOOS == "windows" {
		return `留空自动检测（例如 C:\Program Files\Git\cmd\git.exe）`
	}
	return "留空自动检测（例如 /usr/bin/git）"
}
