package app

import (
	"fmt"
	"slices"
	"strings"

	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/nav"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// remoteSummaryLabel 从 remotes 选择首个 origin Fetch 地址或首个 Fetch 地址，返回顶栏文案。
func remoteSummaryLabel(remotes []git.RemoteAddress, err error) string {
	if err != nil {
		return "远程地址读取失败"
	}
	label := ""
	for _, remote := range remotes {
		if remote.Direction != "fetch" {
			continue
		}
		if remote.Name == "origin" {
			return remote.URL
		}
		if label == "" {
			label = remote.URL
		}
	}
	if label == "" {
		return "未配置远程仓库"
	}
	return label
}

// viewRemoteSummary 在顶栏右侧显示远程地址和仓库详情入口。c 为界面上下文，无返回值。
func (t *RepoTab) viewRemoteSummary(c *ui.Context) {
	tk := theme.TokensOf(c)
	label := t.remoteSummary
	if label == "" {
		label = "正在读取远程地址…"
	}
	ui.Row(c).MaxWidthPercent(42).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		address := ui.Row(c).MinWidth(0).Shrink(1).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Link2()).Size(14, 14).Shrink(0).TextColor(tk.TextTertiary)
			ui.Text(c, label).MinWidth(0).Shrink(1).FontSize(11).SingleLine().Ellipsis("…").TextColor(tk.TextSecondary)
		})
		overlay.Tooltip(c, address, label, overlay.TooltipProps{})
		action.ActionIcon(c.Key("repo-info"), lucide.Info(), action.ActionIconProps{
			Variant:   action.ActionIconBorderless,
			GlyphSize: 15,
			BlockSize: 26,
			Title:     "仓库详情",
		}).Shrink(0).OnClick(t.openRepoInfo)
	})
}

// openRepoInfo 打开详情弹窗并在后台读取当前仓库快照。无输入参数，无返回值。
func (t *RepoTab) openRepoInfo() {
	t.repoInfoSection = repoInfoLocal
	t.repoInfoOpen = true
	if t.repoInfoLoading {
		return
	}
	t.repoInfoLoading = true
	go func() {
		info := t.Client.RepositoryInfo()
		t.App.RunUI(func() {
			if !t.closed {
				t.repoInfo = info
				t.repoInfoLoading = false
			}
		})
	}()
}

const (
	repoInfoLocal = iota
	repoInfoGlobal
	repoInfoOverview
	repoInfoOther
)

// viewRepoInfoModal 渲染当前仓库的只读 Git 信息。c 为界面上下文，无返回值。
func (t *RepoTab) viewRepoInfoModal(c *ui.Context) {
	tk := theme.TokensOf(c)
	feedback.Modal(c.Key("repo-info-dialog:"+t.Client.RepoPath()), &t.repoInfoOpen, feedback.ModalProps{
		Title:    "仓库详情 · " + t.Client.RepoName(),
		Width:    720,
		NoFooter: true,
	}, func() {
		if t.repoInfoLoading {
			ui.Text(c, "正在读取 Git 信息…").TextColor(tk.TextSecondary)
			return
		}
		info := t.repoInfo
		items := []nav.TabItem{
			{Label: "本项目", Icon: lucide.FolderGit2()},
			{Label: "全局", Icon: lucide.Globe()},
			{Label: "仓库概览", Icon: lucide.Info()},
		}
		if slices.ContainsFunc(info.Config, func(entry git.ConfigEntry) bool {
			return repoConfigSection(entry.Scope) == repoInfoOther
		}) {
			items = append(items, nav.TabItem{Label: "其他来源", Icon: lucide.Files()})
		}
		_, height := c.Size()
		nav.Tabs(c.Key("repo-info-sections"), &t.repoInfoSection, nav.TabsProps{
			Items: items, Variant: nav.TabsRounded, Size: theme.Small,
		}, func(section int) {
			ui.Scroll(c).Height(max(160, min(540, height-145))).Children(func() {
				ui.Column(c).Padding(0, 10, 2, 0).Children(func() {
					if section == repoInfoOverview {
						t.viewRepoOverview(c, info)
						return
					}
					viewRepoConfig(c, info, section)
				})
			})
		})
	})
}

// viewRepoOverview 渲染 info 中的工作区、分支和远程地址。无返回值。
func (t *RepoTab) viewRepoOverview(c *ui.Context, info git.RepositoryInfo) {
	tk := theme.TokensOf(c)
	ui.Column(c).Gap(12).Children(func() {
		ui.Column(c).Padding(10, 12).Gap(5).Radius(8).Background(tk.BgLayout).Children(func() {
			viewInfoField(c, "工作区", t.Client.RepoPath())
			viewInfoField(c, "Git 目录", info.GitDir)
			viewInfoField(c, "当前分支", info.Branch.Current)
			viewInfoField(c, "HEAD", info.Head)
			if info.Branch.Upstream != "" {
				viewInfoField(c, "上游分支", fmt.Sprintf("%s（领先 %d，落后 %d）", info.Branch.Upstream, info.Branch.Ahead, info.Branch.Behind))
			}
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "远程仓库").FontSize(13).Bold()
			if info.RemoteErr != nil || len(info.Remotes) == 0 {
				ui.Text(c, remoteSummaryLabel(info.Remotes, info.RemoteErr)).FontSize(12).TextColor(tk.TextSecondary)
			}
			for _, remote := range info.Remotes {
				viewInfoField(c, remote.Name+" · "+remote.Direction, remote.URL)
			}
		})
	})
}

// repoConfigSection 根据 Git 报告的 scope 返回该配置所属的详情标签索引。
func repoConfigSection(scope string) int {
	switch scope {
	case "local", "worktree":
		return repoInfoLocal
	case "global":
		return repoInfoGlobal
	default:
		return repoInfoOther
	}
}

var repoConfigCategories = [...]struct {
	label    string
	sections []string
}{
	{"身份与签名", []string{"user", "author", "committer", "commit", "gpg", "tag"}},
	{"远程与同步", []string{"remote", "branch", "fetch", "pull", "push", "url", "submodule", "clone", "transfer"}},
	{"差异与合并", []string{"diff", "difftool", "merge", "mergetool", "rebase", "rerere", "apply"}},
	{"仓库与工作区", []string{"core", "extensions", "init", "safe", "index", "status", "filter", "lfs"}},
	{"网络与认证", []string{"credential", "http", "https", "ssh", "protocol", "proxy"}},
	{"命令与显示", []string{"alias", "color", "column", "format", "log", "pager", "interactive", "help"}},
	{"配置包含", []string{"include", "includeif"}},
	{"其他配置", nil},
}

// collectRepoConfig 筛选 section 中的 entries，并按配置键的 Git 节名返回分组，保留组内读取顺序。
func collectRepoConfig(entries []git.ConfigEntry, section int) [len(repoConfigCategories)][]git.ConfigEntry {
	var groups [len(repoConfigCategories)][]git.ConfigEntry
	for _, entry := range entries {
		if repoConfigSection(entry.Scope) != section {
			continue
		}
		gitSection, _, _ := strings.Cut(entry.Key, ".")
		index := len(groups) - 1
		for i, category := range repoConfigCategories {
			if slices.Contains(category.sections, gitSection) {
				index = i
				break
			}
		}
		groups[index] = append(groups[index], entry)
	}
	return groups
}

// viewRepoConfig 渲染 info 在 section 标签下的配置分组或读取状态。无返回值。
func viewRepoConfig(c *ui.Context, info git.RepositoryInfo, section int) {
	tk := theme.TokensOf(c)
	if info.ConfigErr != nil {
		ui.Text(c, "Git 配置读取失败").FontSize(12).TextColor(tk.Error)
		return
	}
	groups := collectRepoConfig(info.Config, section)
	if !slices.ContainsFunc(groups[:], func(entries []git.ConfigEntry) bool { return len(entries) > 0 }) {
		ui.Text(c, "此范围暂无 Git 配置").Padding(10, 12).FontSize(12).TextColor(tk.TextSecondary)
		return
	}
	ui.Column(c).Gap(12).Children(func() {
		for i, entries := range groups {
			if len(entries) == 0 {
				continue
			}
			ui.Column(c.Key(i)).Radius(8).Border(1, tk.BorderSecondary).Clip().Children(func() {
				ui.Text(c, repoConfigCategories[i].label).Padding(8, 12).FontSize(12).Bold().Background(tk.BgLayout)
				for _, entry := range entries {
					viewRepoConfigEntry(c, entry)
				}
			})
		}
	})
}

// viewRepoConfigEntry 以键和值两列渲染 entry，并在键下方显示来源。无返回值。
func viewRepoConfigEntry(c *ui.Context, entry git.ConfigEntry) {
	tk := theme.TokensOf(c)
	value := entry.Value
	if value == "" {
		value = "（空值）"
	}
	ui.Row(c).Padding(8, 12).Gap(12).AlignItems(ui.Start).
		BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSecondary).Children(func() {
		ui.Column(c).WidthPercent(45).Shrink(0).Gap(3).Children(func() {
			ui.Text(c, entry.Key).FontSize(12).Bold().Wrap()
			ui.Text(c, entry.Scope+" · "+entry.Origin).FontSize(11).TextColor(tk.TextTertiary).Wrap()
		})
		ui.Text(c, value).Grow(1).Shrink(1).MinWidth(0).FontSize(12).Wrap()
	})
}

// viewInfoField 以两列显示仓库详情字段，label 为名称、value 为内容。无返回值。
func viewInfoField(c *ui.Context, label, value string) {
	tk := theme.TokensOf(c)
	if value == "" {
		value = "—"
	}
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).Width(114).Shrink(0).FontSize(12).Wrap().TextColor(tk.TextSecondary)
		ui.Text(c, value).Grow(1).Shrink(1).MinWidth(0).FontSize(12).Wrap()
	})
}
