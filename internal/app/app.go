package app

import (
	"os"
	"path/filepath"
	"slices"
	"sync"

	"git-me/internal/config"
	"git-me/internal/git"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// App 维护整个 Gitme 应用的多仓库标签页、持久化配置、窗口引用与全局快捷键。
type App struct {
	Store          *config.Store
	Settings       config.Settings
	Win            *mygo.Window
	Tabs           []*RepoTab
	ActiveIndex    int
	RecentMenuOpen bool
	GlobalError    string
	uiMu           sync.Mutex
	pendingUI      []func()
	updates        *updateControls
}

// NewApp 创建并初始化 Gitme 顶层应用状态，自动恢复上次打开的仓库标签页或命令行传入的目录。
// 输入参数 initialPaths 为命令行传入的初始目录路径列表；返回 *App 实例。
func NewApp(initialPaths []string) *App {
	store := config.NewStore("")
	cfg, _ := store.Load()

	a := &App{
		Store:       store,
		Settings:    cfg,
		ActiveIndex: 0,
	}

	// 1. 优先恢复上次未关闭的标签页
	for _, p := range cfg.OpenTabs {
		_ = a.OpenRepo(p, false)
	}
	// 2. 打开命令行传入的仓库路径
	for _, p := range initialPaths {
		_ = a.OpenRepo(p, true)
	}
	// 3. 如果没有任何标签页，尝试打开当前工作目录（如果是 Git 仓库）
	if len(a.Tabs) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			_ = a.OpenRepo(cwd, true)
		}
	}
	// 4. 恢复上次激活的标签页索引
	if cfg.ActiveTab != "" && len(initialPaths) == 0 {
		for i, tab := range a.Tabs {
			if tab.Client.RepoPath() == filepath.Clean(cfg.ActiveTab) {
				a.ActiveIndex = i
				break
			}
		}
	}
	return a
}

// SetWindow 绑定主窗口实例并注册拖放文件夹事件。
// 输入参数 win 为 MyGo 主窗口指针；无返回值。
func (a *App) SetWindow(win *mygo.Window) {
	a.uiMu.Lock()
	a.Win = win
	if win == nil {
		a.uiMu.Unlock()
		return
	}
	for _, fn := range a.pendingUI {
		win.Update(fn)
	}
	a.pendingUI = nil
	a.uiMu.Unlock()
	win.OnFileDrop(func(e *mygo.FileDropEvent) {
		a.RunUI(func() {
			for _, p := range e.Paths {
				_ = a.OpenRepo(p, true)
			}
		})
	})
	a.startUpdateChecks(win)
	a.installUpdateMenu(win)
}

// RunUI 将状态更新交给窗口主线程，窗口绑定前暂存至首次渲染或窗口绑定。
// 输入参数 fn 为要在主线程运行的状态修改函数；无返回值。
func (a *App) RunUI(fn func()) {
	a.uiMu.Lock()
	defer a.uiMu.Unlock()
	if a.Win != nil {
		a.Win.Update(fn)
		return
	}
	a.pendingUI = append(a.pendingUI, fn)
}

// ActiveTab 返回当前处于激活状态的仓库标签页指针（无打开仓库时返回 nil）。
// 无输入参数，返回 *RepoTab 或 nil。
func (a *App) ActiveTab() *RepoTab {
	if a.ActiveIndex < 0 || a.ActiveIndex >= len(a.Tabs) {
		return nil
	}
	return a.Tabs[a.ActiveIndex]
}

// OpenRepo 验证并打开指定路径所属的 Git 仓库；若已打开则直接切换到该标签页。
// 输入参数 dirPath 为文件夹或文件路径，activate 表示是否立即切换到该标签页；返回可能的错误。
func (a *App) OpenRepo(dirPath string, activate bool) error {
	client, err := git.NewClient(dirPath)
	if err != nil {
		a.GlobalError = err.Error()
		return err
	}
	a.GlobalError = ""
	repoRoot := client.RepoPath()

	for i, existing := range a.Tabs {
		if existing.Client.RepoPath() == repoRoot {
			if activate {
				a.ActiveIndex = i
				a.SaveSettings()
			}
			return nil
		}
	}

	tab := NewRepoTab(a, client)
	a.Tabs = append(a.Tabs, tab)
	if activate || len(a.Tabs) == 1 {
		a.ActiveIndex = len(a.Tabs) - 1
	}
	config.AddRecentRepo(&a.Settings, repoRoot)
	a.SaveSettings()
	return nil
}

// CloseTab 关闭指定索引位置的仓库标签页并保存配置。
// 输入参数 idx 为标签页索引；无返回值。
func (a *App) CloseTab(idx int) {
	if idx < 0 || idx >= len(a.Tabs) {
		return
	}
	a.Tabs[idx].Close()
	a.Tabs = slices.Delete(a.Tabs, idx, idx+1)
	if len(a.Tabs) == 0 {
		a.ActiveIndex = 0
	} else if a.ActiveIndex >= len(a.Tabs) {
		a.ActiveIndex = len(a.Tabs) - 1
	}
	a.SaveSettings()
}

// PickRepoFolder 弹出系统原生文件夹选择对话框，供用户选择并打开一个 Git 仓库目录。
// 无输入参数，无返回值。
func (a *App) PickRepoFolder() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.Win,
			Title:     "选择 Git 仓库目录",
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.RunUI(func() {
			for _, p := range paths {
				_ = a.OpenRepo(p, true)
			}
		})
	}()
}

// SaveSettings 将当前打开的标签页列表、激活标签页和视图偏好持久化到磁盘。
// 无输入参数，无返回值。
func (a *App) SaveSettings() {
	a.Settings.OpenTabs = a.Settings.OpenTabs[:0]
	for _, tab := range a.Tabs {
		a.Settings.OpenTabs = append(a.Settings.OpenTabs, tab.Client.RepoPath())
	}
	if cur := a.ActiveTab(); cur != nil {
		a.Settings.ActiveTab = cur.Client.RepoPath()
	} else {
		a.Settings.ActiveTab = ""
	}
	_ = a.Store.Save(a.Settings)
}

// ApplyDiffLayoutToAllTabs 在用户切换“左右分栏 / 上下合并”时同步更新所有已加载的 DiffState。
// 无输入参数，无返回值。
func (a *App) ApplyDiffLayoutToAllTabs() {
	for _, tab := range a.Tabs {
		for _, entry := range tab.DiffEntries {
			entry.syncLayoutFromSettings(a.Settings.UnifiedDiff)
		}
	}
	a.SaveSettings()
}

// View 是提供给 MyGo 窗口渲染的顶层视图入口函数，启用 Lobe UI 主题 Tokens。
// 输入参数 c 为窗口的 *ui.Context；无返回值。
func (a *App) View(c *ui.Context) {
	a.uiMu.Lock()
	pending := a.pendingUI
	a.pendingUI = nil
	a.uiMu.Unlock()
	for _, fn := range pending {
		fn()
	}
	theme.Apply(c)
	tk := theme.TokensOf(c)

	// 全局窗口快捷键：Cmd+O 打开仓库，Cmd+R 刷新当前仓库，Cmd+S 保存当前微调文件
	c.OnWindowShortcut(ui.Cmd, 'o', a.PickRepoFolder)
	c.OnWindowShortcut(ui.Cmd, 'r', func() {
		if tab := a.ActiveTab(); tab != nil {
			tab.refreshManually()
		}
	})
	c.OnWindowShortcut(ui.Cmd, 's', func() {
		if tab := a.ActiveTab(); tab != nil {
			if SidebarMode(tab.SidebarMode) == ModeProjectFiles && tab.IsProjectFileEdited() {
				tab.SaveCurrentProjectFile()
				return
			}
			if entry, ok := tab.DiffEntries[tab.SelectedFileKey]; ok && entry.IsEdited() {
				tab.SaveEditedFile(entry)
			}
		}
	})

	root := ui.Column(c).Fill().Background(tk.BgContainer)
	if dropped := root.DroppedFiles(); len(dropped) > 0 {
		for _, p := range dropped {
			_ = a.OpenRepo(p, true)
		}
	}

	root.Children(func() {
		// 1. 顶部多仓库标签栏与全局导航栏
		a.viewTopBar(c)

		curTab := a.ActiveTab()
		if curTab == nil {
			a.viewEmptyWelcome(c)
			return
		}

		// 2. 仓库级工具栏（分支管理、Pull/Push/Fetch、状态横幅）
		curTab.viewRepoToolbar(c)

		// 3. 主分栏视图：左侧边栏 + 右侧主视图
		prevWidth := a.Settings.SidebarWidth
		ui.Split(c, &a.Settings.SidebarWidth, func() {
			curTab.viewSidebar(c)
		}, func() {
			curTab.viewDiffPane(c)
		}).SplitReset(300).Grow(1)

		if a.Settings.SidebarWidth != prevWidth {
			a.SaveSettings()
		}

		// 4. 底部状态栏
		curTab.viewStatusBar(c)

		// 5. 新建分支弹窗
		curTab.viewNewBranchModal(c)
	})
}

// viewEmptyWelcome 在尚未打开任何 Git 仓库时使用 Lobe UI 组件渲染轻量引导页。
// 输入参数 c 为 *ui.Context；无返回值。
func (a *App) viewEmptyWelcome(c *ui.Context) {
	tk := theme.TokensOf(c)
	ui.Column(c).Grow(1).Center().Padding(32).Gap(16).Children(func() {
		display.Empty(c, display.EmptyProps{
			Icon:        lucide.FolderGit2(),
			Title:       "欢迎使用 Gitme",
			Description: "轻量级 Git 图形界面工具",
			Action: func() {
				if action.Button(c, "+ 打开目录", action.ButtonProps{
					Variant: action.Primary,
					Size:    theme.Middle,
				}).Clicked() {
					a.PickRepoFolder()
				}
			},
		})

		if a.GlobalError != "" {
			ui.Text(c, a.GlobalError).FontSize(12).TextColor(tk.Error)
		}

		if len(a.Settings.RecentRepos) > 0 {
			ui.Column(c).Width(460).Padding(12).Radius(8).Border(1, tk.BorderSecondary).Background(tk.BgLayout).Gap(6).Children(func() {
				ui.Text(c, "最近打开的仓库").FontSize(12).Bold().TextColor(tk.TextSecondary)
				for _, repoPath := range a.Settings.RecentRepos {
					p := repoPath
					row := ui.Row(c.Key("welcome-recent:"+p)).
						Padding(6, 10).
						Radius(6).
						AlignItems(ui.Center).
						Justify(ui.SpaceBetween).
						Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(tk.FillTertiary)
					}
					if row.Clicked() {
						_ = a.OpenRepo(p, true)
					}
					row.Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							display.Icon(c, lucide.FolderGit2(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
							ui.Text(c, filepath.Base(p)).FontSize(13).Bold()
						})
						ui.Text(c, p).FontSize(11).TextColor(tk.TextTertiary)
					})
				}
			})
		}
	})
}
