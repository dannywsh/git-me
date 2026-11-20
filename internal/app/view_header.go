package app

import (
	"fmt"
	"path/filepath"
	"time"

	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// viewTopBar 渲染窗口最顶部的多仓库标签栏（Tabs）、打开目录按钮以及 Lobe 下拉最近仓库菜单。
// 输入参数 c 为 *ui.Context；无返回值。
func (a *App) viewTopBar(c *ui.Context) {
	tk := theme.TokensOf(c)

	ui.Row(c).
		Padding(4, 10).
		Gap(6).
		AlignItems(ui.Center).
		Background(tk.BgLayout).
		BorderWidth(0, 0, 1, 0).
		BorderColor(tk.BorderSecondary).
		Children(func() {
			// 1. 已打开的仓库标签页列表
			ui.ScrollHorizontal(c).Grow(1).Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					for i, tab := range a.Tabs {
						idx := i
						repoPath := tab.Client.RepoPath()
						repoName := tab.Client.RepoName()
						isActive := idx == a.ActiveIndex

						tabBox := ui.Row(c.Key("repotab:"+repoPath)).
							Padding(3, 8).
							Gap(6).
							Radius(6).
							AlignItems(ui.Center).
							Cursor(ui.CursorPointer)

						if isActive {
							tabBox.Background(tk.BgContainer).Border(1, tk.PrimaryBorder)
						} else if tabBox.Hovered() {
							tabBox.Background(tk.FillTertiary).Border(1, tk.BorderSecondary)
						} else {
							tabBox.Border(1, tk.BorderSecondary)
						}

						if tabBox.Clicked() {
							a.ActiveIndex = idx
							a.SaveSettings()
						}

						tabBox.Children(func() {
							display.Icon(c, lucide.FolderGit2(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
							ui.Text(c, repoName).FontSize(12).Bold()
							changeCount := len(tab.StagedFiles) + len(tab.UnstagedFiles)
							if changeCount > 0 {
								ui.Badge(c, fmt.Sprintf("%d", changeCount))
							}
							if action.ActionIcon(c, lucide.X(), action.ActionIconProps{
								Variant:   action.ActionIconBorderless,
								GlyphSize: 11,
								BlockSize: 18,
								Title:     "关闭此仓库标签页",
							}).Clicked() {
								a.CloseTab(idx)
							}
						})
					}
				})
			})

			// 2. 打开新仓库文件夹按钮（紧凑尺寸）
			if action.Button(c, "+ 打开目录", action.ButtonProps{
				Variant: action.Default,
				Size:    theme.Small,
			}).Clicked() {
				a.PickRepoFolder()
			}

			// 3. 最近打开的仓库 Lobe DropdownMenu
			if len(a.Settings.RecentRepos) > 0 {
				var items []overlay.MenuItem
				curPath := ""
				if cur := a.ActiveTab(); cur != nil {
					curPath = cur.Client.RepoPath()
				}
				for _, rp := range a.Settings.RecentRepos {
					icon := lucide.Folder()
					if rp == curPath {
						icon = lucide.Check()
					}
					items = append(items, overlay.MenuItem{
						Key:   rp,
						Label: filepath.Base(rp),
						Extra: rp,
						Icon:  icon,
					})
				}
				chosen := overlay.DropdownMenu(c, &a.RecentMenuOpen, overlay.DropdownMenuProps{
					Items: items,
				}, func(trig ui.Element) {
					action.StyleButton(c, trig, "最近仓库 ▾", action.ButtonProps{
						Variant: action.Default,
						Size:    theme.Small,
					})
				})
				if chosen != "" {
					_ = a.OpenRepo(chosen, true)
				}
			}
		})
}

// viewRepoToolbar 渲染当前激活仓库的紧凑工具栏：Lobe 分支下拉菜单、图标化新建分支、Pull/Push/Fetch、图标化刷新按钮及状态提示条。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewRepoToolbar(c *ui.Context) {
	tk := theme.TokensOf(c)

	ui.Row(c).
		Padding(4, 10).
		Gap(6).
		AlignItems(ui.Center).
		Background(tk.BgContainer).
		BorderWidth(0, 0, 1, 0).
		BorderColor(tk.BorderSecondary).
		Children(func() {
			// 分支选择 Lobe DropdownMenu
			branchLabel := "分支: " + t.BranchInfo.Current
			if t.BranchInfo.Current == "" {
				branchLabel = "分支: HEAD"
			}
			menuItems := []overlay.MenuItem{
				{
					Key:   "__new_branch__",
					Label: "新建并切换分支...",
					Icon:  lucide.GitBranchPlus(),
				},
			}
			if len(t.BranchInfo.Branches) > 0 {
				menuItems = append(menuItems, overlay.MenuItem{Divider: true})
			}
			for _, bName := range t.BranchInfo.Branches {
				ic := lucide.GitBranch()
				if bName == t.BranchInfo.Current {
					ic = lucide.Check()
				}
				menuItems = append(menuItems, overlay.MenuItem{
					Key:   "branch:" + bName,
					Label: bName,
					Icon:  ic,
				})
			}

			chosen := overlay.DropdownMenu(c, &t.BranchMenuOpen, overlay.DropdownMenuProps{
				Items: menuItems,
			}, func(trig ui.Element) {
				action.StyleButton(c, trig, branchLabel+" ▾", action.ButtonProps{
					Variant: action.Default,
					Size:    theme.Small,
				})
			})
			if chosen == "__new_branch__" {
				t.NewBranchName = ""
				t.BranchErrMsg = ""
				t.NewBranchModal = true
			} else if len(chosen) > 7 && chosen[:7] == "branch:" {
				branch := chosen[7:]
				if branch != t.BranchInfo.Current {
					t.RunAsyncTask("正在切换分支到 "+branch, func() (string, error) {
						if err := t.Client.CheckoutBranch(branch); err != nil {
							return "", err
						}
						return "已切换到分支 " + branch, nil
					})
				}
			}

			// 新建分支紧凑图标按钮
			if action.ActionIcon(c, lucide.GitBranchPlus(), action.ActionIconProps{
				Variant: action.ActionIconOutlined,
				Size:    theme.Small,
				Title:   "新建并切换分支",
			}).Clicked() {
				t.NewBranchName = ""
				t.BranchErrMsg = ""
				t.NewBranchModal = true
			}

			layout.Divider(c, layout.DividerProps{Vertical: true})

			// 远程同步操作：保留常用英文 Fetch / Pull / Push，使用带精细 SVG 图标且高度与字体严格一致的按钮
			if renderIconTextButton(c, lucide.CloudDownload(), "Fetch", t.Busy).Clicked() {
				t.RunAsyncTask("正在 Fetch 远程更新", func() (string, error) {
					if _, err := t.Client.Fetch(); err != nil {
						return "", err
					}
					return "Fetch 完成", nil
				})
			}

			pullLabel := "Pull"
			if t.BranchInfo.Behind > 0 {
				pullLabel = fmt.Sprintf("Pull (%d)", t.BranchInfo.Behind)
			}
			if renderIconTextButton(c, lucide.ArrowDownToLine(), pullLabel, t.Busy).Clicked() {
				t.RunAsyncTask("正在 Pull 拉取更新", func() (string, error) {
					if _, err := t.Client.Pull(); err != nil {
						return "", err
					}
					return "Pull 更新成功", nil
				})
			}

			pushLabel := "Push"
			if t.BranchInfo.Ahead > 0 {
				pushLabel = fmt.Sprintf("Push (%d)", t.BranchInfo.Ahead)
			}
			if renderIconTextButton(c, lucide.ArrowUpFromLine(), pushLabel, t.Busy).Clicked() {
				t.RunAsyncTask("正在 Push 推送提交", func() (string, error) {
					if _, err := t.Client.Push(); err != nil {
						return "", err
					}
					return "Push 推送成功", nil
				})
			}

			// 刷新按钮简化为 Lobe ActionIcon
			if action.ActionIcon(c, lucide.RefreshCw(), action.ActionIconProps{
				Variant: action.ActionIconOutlined,
				Size:    theme.Small,
				Title:   "刷新仓库状态 (Cmd+R)",
			}).Disabled(t.Busy).Clicked() {
				t.refreshManually()
			}
			if !t.lastRefreshAt.IsZero() {
				label, nextUpdate := formatRefreshTime(t.lastRefreshAt, c.Now())
				ui.Text(c, label).FontSize(12).TextColor(tk.TextTertiary).SingleLine()
				if nextUpdate > 0 {
					c.After(nextUpdate)
				}
			}

			// 旋转加载图标与状态横幅消息
			if t.Busy {
				display.Icon(c, lucide.RefreshCw(), display.IconProps{
					Size: display.IconSmall,
					Spin: true,
				}).TextColor(tk.Primary)
			}
			if t.StatusBanner != "" {
				color := tk.TextSecondary
				if t.IsBannerError {
					color = tk.Error
				}
				ui.Text(c, t.StatusBanner).FontSize(12).TextColor(color).Grow(1)
			} else {
				ui.Spacer(c)
			}
		})
}

// formatRefreshTime 根据刷新时间 refreshedAt 和当前时间 now 生成标签及距下次更新的间隔。
// 满一小时后返回本地日期和时间，更新间隔为零。
func formatRefreshTime(refreshedAt, now time.Time) (string, time.Duration) {
	age := max(now.Sub(refreshedAt), 0)
	switch {
	case age < time.Second:
		return "刚刚", refreshedAt.Add(time.Second).Sub(now)
	case age < time.Minute:
		return fmt.Sprintf("%d 秒前", age/time.Second), time.Second - age%time.Second
	case age < time.Hour:
		return fmt.Sprintf("%d 分钟前", age/time.Minute), time.Minute - age%time.Minute
	default:
		return refreshedAt.Local().Format("2006-01-02 15:04"), 0
	}
}

// viewStatusBar 渲染窗口最底部的轻量状态栏（使用原生字体展示实时监听状态，保持与左侧已暂存统计字体大小完全一致）。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewStatusBar(c *ui.Context) {
	tk := theme.TokensOf(c)

	ui.Row(c).
		Padding(3, 12).
		Gap(12).
		AlignItems(ui.Center).
		Background(tk.BgLayout).
		BorderWidth(1, 0, 0, 0).
		BorderColor(tk.BorderSecondary).
		Children(func() {
			ui.Text(c, t.Client.RepoPath()).FontSize(11).TextColor(tk.TextTertiary)
			if t.BranchInfo.Upstream != "" {
				ui.Textf(c, "上游: %s (↑%d ↓%d)", t.BranchInfo.Upstream, t.BranchInfo.Ahead, t.BranchInfo.Behind).
					FontSize(11).
					TextColor(tk.TextTertiary)
			}
			ui.Spacer(c)
			ui.Textf(c, "已暂存: %d  |  工作区变更: %d", len(t.StagedFiles), len(t.UnstagedFiles)).
				FontSize(11).
				TextColor(tk.TextTertiary)
			ui.Text(c, "● 实时监听中").FontSize(11).TextColor(tk.Success)
		})
}

// viewNewBranchModal 使用 Lobe UI 的 feedback.Modal 与 field.Input 渲染新建并切换分支弹窗。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewNewBranchModal(c *ui.Context) {
	tk := theme.TokensOf(c)

	createFn := func() {
		validName, err := git.ValidateBranchName(t.NewBranchName)
		if err != nil {
			t.BranchErrMsg = err.Error()
			return
		}
		t.NewBranchModal = false
		t.RunAsyncTask("正在创建分支 "+validName, func() (string, error) {
			if err := t.Client.CreateAndCheckoutBranch(validName); err != nil {
				return "", err
			}
			return "已创建并切换到分支 " + validName, nil
		})
	}

	okClicked := feedback.Modal(c, &t.NewBranchModal, feedback.ModalProps{
		Title:      "新建并切换分支",
		Width:      380,
		OkText:     "创建分支",
		CancelText: "取消",
	}, func() {
		ui.Column(c).Gap(8).Children(func() {
			inp := field.Input(c, &t.NewBranchName, field.InputProps{
				Placeholder: "例如: feat/agent-review",
				Prefix:      lucide.GitBranch(),
				Size:        theme.Middle,
				Invalid:     t.BranchErrMsg != "",
			}).AutoFocus()
			if inp.Submitted() {
				createFn()
			}
			if t.BranchErrMsg != "" {
				ui.Text(c, t.BranchErrMsg).FontSize(12).TextColor(tk.Error)
			}
		})
	})

	if okClicked {
		createFn()
	}
}

// renderIconTextButton 使用原生 ui.ButtonBase 渲染一个带 13x13 矢量图标与 12px 文本的紧凑按钮（固定高度 24 DIP，图标与文字垂直居中且视觉比例一致）。
// 输入参数 c 为 *ui.Context，iconSvg 为矢量图标，label 为按钮文字，disabled 表示是否禁用；返回按钮 ui.Element。
func renderIconTextButton(c *ui.Context, iconSvg *ui.SVG, label string, disabled bool) ui.Element {
	tk := theme.TokensOf(c)
	btn := ui.ButtonBase(c).
		Height(24).
		Padding(0, 8).
		Gap(5).
		Radius(6).
		AlignItems(ui.Center).
		Border(1, tk.Border).
		Background(tk.BgContainer).
		Disabled(disabled)

	if !disabled {
		btn.Cursor(ui.CursorPointer)
		if btn.Hovered() {
			btn.Background(tk.FillTertiary).BorderColor(tk.BorderSecondary)
		}
	}

	textColor := tk.Text
	iconColor := tk.TextSecondary
	if disabled {
		textColor = tk.TextDisabled
		iconColor = tk.TextDisabled
	}

	btn.Children(func() {
		ui.Icon(c, iconSvg).Size(13, 13).TextColor(iconColor)
		ui.Text(c, label).FontSize(12).TextColor(textColor).SingleLine()
	})
	return btn
}
