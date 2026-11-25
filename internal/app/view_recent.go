package app

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// viewRecentRepoSection 在侧栏底部渲染可折叠的历史列表，activePath 用于标记当前仓库。无返回值。
func (a *App) viewRecentRepoSection(c *ui.Context, activePath string) {
	tk := theme.TokensOf(c)
	ui.Column(c.Key("recent-repos")).Shrink(0).Padding(4, 6).
		BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSecondary).Children(func() {
		header := ui.ButtonBase(c).Label("最近仓库").Expanded(a.recentExpanded).Height(28).Padding(0, 6).Gap(6).Radius(6).
			AlignItems(ui.Center).Cursor(ui.CursorPointer).
			OnClick(func() { a.recentExpanded = !a.recentExpanded })
		if header.Hovered() {
			header.Background(tk.FillTertiary)
		}
		header.Children(func() {
			ui.Icon(c, lucide.History()).Size(13, 13).TextColor(tk.TextSecondary)
			ui.Text(c, "最近仓库").FontSize(12).Bold()
			ui.Text(c, fmt.Sprint(len(a.Settings.RecentRepos))).FontSize(11).TextColor(tk.TextTertiary)
			ui.Spacer(c)
			chevron := lucide.ChevronRight()
			if a.recentExpanded {
				chevron = lucide.ChevronDown()
			}
			ui.Icon(c, chevron).Size(13, 13).TextColor(tk.TextSecondary)
		})
		if a.recentExpanded {
			a.viewRecentRepos(c, activePath)
		}
		if a.GlobalError != "" {
			ui.Text(c, a.GlobalError).FontSize(11).MaxLines(2).TextColor(tk.Error)
		}
	})
}

// viewRecentRepos 渲染可滚动的历史记录，支持打开仓库和移除单条记录。activePath 可为空，无返回值。
func (a *App) viewRecentRepos(c *ui.Context, activePath string) {
	tk := theme.TokensOf(c)
	if len(a.Settings.RecentRepos) == 0 {
		ui.Text(c, "暂无最近仓库").Padding(6, 8).FontSize(11).TextColor(tk.TextTertiary)
		return
	}
	ui.Scroll(c).Height(float32(min(len(a.Settings.RecentRepos), 4) * 44)).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for _, path := range a.Settings.RecentRepos {
				row := ui.Row(c.Key("recent:"+path)).Height(42).Gap(2).Radius(6).
					Padding(0, 5, 0, 0).AlignItems(ui.Center)
				reveal := row.Hovered() || row.FocusWithin()
				if reveal || path == activePath {
					row.Background(tk.FillTertiary)
				}
				row.Children(func() {
					open := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).Height(40).
						Padding(0, 7).Gap(7).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).
						Label(path).OnClick(func() { _ = a.OpenRepo(path, true) }).Children(func() {
						icon, color := lucide.FolderGit2(), tk.TextSecondary
						if path == activePath {
							icon, color = lucide.Check(), tk.Primary
						}
						ui.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(color)
						ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, filepath.Base(path)).FontSize(12).Bold().SingleLine().Ellipsis("…")
							ui.Text(c, path).FontSize(10).TextColor(tk.TextTertiary).SingleLine().Ellipsis("…")
						})
					})
					overlay.Tooltip(c, open, path, overlay.TooltipProps{})
					remove := action.ActionIcon(c.Key("remove"), lucide.X(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 12,
						BlockSize: 22,
						Title:     "从最近仓库移除 " + filepath.Base(path),
					}).Shrink(0).OnClick(func() { a.removeRecentRepo(path) })
					if !reveal && !remove.Focused() {
						remove.Opacity(0)
					}
				})
			}
		})
	})
}
