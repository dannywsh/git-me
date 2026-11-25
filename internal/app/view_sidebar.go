package app

import (
	"fmt"
	"strings"

	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// viewSidebar 渲染左侧边栏，采用 Lobe UI 的 Segmented、SearchBar、ActionIcon 与紧凑圆点状态指示器。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewSidebar(c *ui.Context) {
	tk := theme.TokensOf(c)

	ui.Column(c).
		Fill().
		Background(tk.BgLayout).
		BorderWidth(0, 1, 0, 0).
		BorderColor(tk.BorderSecondary).
		Children(func() {
			// 1. 顶部模式切换 + 搜索栏与平铺/目录树图标切换
			ui.Column(c).Padding(8, 8).Gap(6).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
				prevMode := t.SidebarMode
				action.Segmented(c, &t.SidebarMode, []string{"代码变更", "提交历史", "项目文件"}, action.SegmentedProps{
					Size:  theme.Small,
					Block: true,
				})
				if t.SidebarMode != prevMode {
					switch SidebarMode(t.SidebarMode) {
					case ModeWorkingChanges:
						files := t.ActiveFileList()
						if len(files) > 0 {
							t.SelectFile(files[0])
						}
					case ModeCommitHistory:
						if t.SelectedCommitHash == "" && len(t.Commits) > 0 {
							t.SelectCommit(t.Commits[0].Hash)
						}
					case ModeProjectFiles:
						if len(t.ProjectFiles) == 0 {
							t.ReloadProjectFileListAsync()
						} else if t.SelectedProjectPath == "" {
							t.OpenProjectFile(t.ProjectFiles[0], 0, "")
						}
					}
				}

				if SidebarMode(t.SidebarMode) == ModeProjectFiles {
					// 【项目文件】自适应搜索框 + 目录树切换图标（跟随分栏拖动自适应宽度）
					ui.Row(c).WidthPercent(100).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Grow(1).Shrink(1).ClipX().Children(func() {
							if ui.SearchField(c, &t.ProjectSearchQuery).
								Placeholder("搜索文件名或代码...").
								FontSize(12).
								Grow(1).
								Shrink(1).
								Changed() {
								t.TriggerCodeSearchAsync()
							}
						})
						t.viewTreeToggleIcon(c)
					})
					ui.Row(c).WidthPercent(100).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
						if renderCompactCheckbox(c, &t.SearchCaseSensitive, "区分大小写") {
							t.TriggerCodeSearchAsync()
						}
						if renderCompactCheckbox(c, &t.SearchIncludeIgnore, "包含忽略文件") {
							t.ReloadProjectFileListAsync()
							t.TriggerCodeSearchAsync()
						}
					})
				} else {
					// 【代码变更】与【提交历史】自适应过滤栏 + 目录树切换图标
					ui.Row(c).WidthPercent(100).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Row(c).Grow(1).Shrink(1).ClipX().Children(func() {
							ui.SearchField(c, &t.SearchQuery).
								Placeholder("按文件名过滤...").
								FontSize(12).
								Grow(1).
								Shrink(1)
						})
						t.viewTreeToggleIcon(c)
					})
				}
			})

			// 2. 根据所选模式渲染对应面板
			switch SidebarMode(t.SidebarMode) {
			case ModeWorkingChanges:
				t.viewWorkingChangesPanel(c)
			case ModeCommitHistory:
				t.viewCommitHistoryPanel(c)
			case ModeProjectFiles:
				t.viewProjectFilesPanel(c)
			}
			t.App.viewRecentRepoSection(c, t.Client.RepoPath())
		})
}

// viewTreeToggleIcon 渲染用于切换“目录树 / 平铺列表”的紧凑 Lobe ActionIcon 图标按钮。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewTreeToggleIcon(c *ui.Context) {
	icon := lucide.List()
	title := "当前为平铺列表，点击切换为目录树"
	if t.App.Settings.TreeMode {
		icon = lucide.FolderTree()
		title = "当前为目录树，点击切换为平铺列表"
	}
	if action.ActionIcon(c, icon, action.ActionIconProps{
		Variant: action.ActionIconOutlined,
		Size:    theme.Small,
		Active:  t.App.Settings.TreeMode,
		Title:   title,
	}).Clicked() {
		t.App.Settings.TreeMode = !t.App.Settings.TreeMode
		t.App.SaveSettings()
	}
}

// viewWorkingChangesPanel 渲染“代码变更”模式下的原生 Commit 提交区、已暂存分组与工作区未暂存分组。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewWorkingChangesPanel(c *ui.Context) {
	tk := theme.TokensOf(c)

	// Commit 输入与按钮区（全部使用原生组件，固定尺寸与字号，杜绝点击后样式错乱）
	ui.Column(c).Padding(8, 8).Gap(6).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
		ui.TextArea(c, &t.CommitMessage).
			Placeholder("输入提交说明...").
			FontSize(12).
			Height(60)

		ui.Row(c).Height(22).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(6).Children(func() {
			if renderCompactCheckbox(c, &t.AmendCommit, "修正上次提交") && t.AmendCommit && strings.TrimSpace(t.CommitMessage) == "" {
				go func() {
					if msg, err := t.Client.LastCommitMessage(); err == nil && msg != "" {
						t.App.RunUI(func() {
							if strings.TrimSpace(t.CommitMessage) == "" {
								t.CommitMessage = msg
							}
						})
					}
				}()
			}

			canCommit := !t.Busy && strings.TrimSpace(t.CommitMessage) != "" && (len(t.StagedFiles) > 0 || t.AmendCommit)
			commitLabel := fmt.Sprintf("提交 (%d)", len(t.StagedFiles))
			if renderCompactPrimaryButton(c, lucide.Check(), commitLabel, !canCommit).Clicked() {
				msg := strings.TrimSpace(t.CommitMessage)
				amend := t.AmendCommit
				t.RunAsyncTask("正在执行提交", func() (string, error) {
					if err := t.Client.Commit(msg, amend); err != nil {
						return "", err
					}
					t.App.RunUI(func() {
						t.CommitMessage = ""
						t.AmendCommit = false
					})
					return "", nil
				})
			}
		})
	})

	// 变更文件列表滚动区
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(6, 6).Gap(8).Children(func() {
			stagedFiltered := FilterFiles(t.StagedFiles, t.SearchQuery)
			unstagedFiltered := FilterFiles(t.UnstagedFiles, t.SearchQuery)

			// 分组 1：已暂存
			t.viewFileGroupHeader(c, "已暂存", len(stagedFiltered), &t.StagedOpen, func() {
				if len(t.StagedFiles) > 0 {
					if action.ActionIcon(c, lucide.Minus(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 13,
						BlockSize: 20,
						Title:     "全部取消暂存",
					}).Disabled(t.Busy).Clicked() {
						t.RunAsyncTask("正在取消全部暂存", func() (string, error) {
							if err := t.Client.UnstageAll(); err != nil {
								return "", err
							}
							return "已取消全部暂存文件", nil
						})
					}
				}
			})
			if t.StagedOpen {
				if len(stagedFiltered) == 0 {
					ui.Text(c, "  暂无已暂存文件").FontSize(11).TextColor(tk.TextTertiary)
				} else {
					t.viewFileCollection(c, "staged", stagedFiltered, true)
				}
			}

			// 分组 2：未暂存更改
			t.viewFileGroupHeader(c, "未暂存更改", len(unstagedFiltered), &t.UnstagedOpen, func() {
				if len(t.UnstagedFiles) > 0 {
					if action.ActionIcon(c, lucide.RotateCcw(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 13,
						BlockSize: 20,
						Danger:    true,
						Title:     "放弃工作区全部修改",
					}).Disabled(t.Busy).Clicked() {
						t.RunAsyncTask("正在放弃工作区全部修改", func() (string, error) {
							if err := t.Client.DiscardAll(); err != nil {
								return "", err
							}
							return "已放弃工作区全部修改", nil
						})
					}
					if action.ActionIcon(c, lucide.Plus(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 13,
						BlockSize: 20,
						Title:     "全部暂存",
					}).Disabled(t.Busy).Clicked() {
						t.RunAsyncTask("正在暂存全部更改", func() (string, error) {
							if err := t.Client.StageAll(); err != nil {
								return "", err
							}
							return "已暂存全部文件", nil
						})
					}
				}
			})
			if t.UnstagedOpen {
				if len(unstagedFiltered) == 0 {
					ui.Text(c, "  工作区干净，无未暂存修改").FontSize(11).TextColor(tk.TextTertiary)
				} else {
					t.viewFileCollection(c, "unstaged", unstagedFiltered, true)
				}
			}
		})
	})
}

// viewProjectFilesPanel 渲染“项目文件”模式：未输入搜索词时展示项目所有文件树/列表；输入搜索词时同时展示匹配的文件名与代码行。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewProjectFilesPanel(c *ui.Context) {
	tk := theme.TokensOf(c)
	q := strings.TrimSpace(t.ProjectSearchQuery)
	statusMap := t.WorkingStatusMap()

	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(6, 6).Gap(8).Children(func() {
			if q == "" {
				ui.Row(c).Padding(2, 4).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					ui.Text(c, "项目全部文件").FontSize(12).Bold().TextColor(tk.TextSecondary)
					ui.Badge(c, fmt.Sprintf("%d", len(t.ProjectFiles)))
				})
				if len(t.ProjectFiles) == 0 {
					ui.Text(c, "  正在扫描或项目中暂无文件").FontSize(11).TextColor(tk.TextTertiary)
					return
				}
				if !t.App.Settings.TreeMode {
					ui.Column(c).Gap(1).Children(func() {
						for _, p := range t.ProjectFiles {
							t.viewProjectFileRow(c, "proj-flat", p, p, 0, statusMap[p])
						}
					})
				} else {
					nodes := BuildProjectPathTree(t.Client.RepoName(), t.ProjectFiles, statusMap)
					ui.Column(c).Gap(1).Children(func() {
						for _, n := range nodes {
							t.viewProjectTreeNode(c, "proj-tree", n, 0)
						}
					})
				}
				return
			}

			// 输入了搜索词 -> 同时展示“文件名匹配”和“代码内容匹配”
			matchedFiles := FilterPaths(t.ProjectFiles, q, t.SearchCaseSensitive)

			openFilesSec := t.TreeOpenBool("search:sec:files")
			t.viewFileGroupHeader(c, "文件名匹配", len(matchedFiles), openFilesSec, func() {})
			if *openFilesSec {
				if len(matchedFiles) == 0 {
					ui.Text(c, "  没有文件名包含该关键词").FontSize(11).TextColor(tk.TextTertiary)
				} else {
					limit := min(len(matchedFiles), 80)
					ui.Column(c).Gap(1).Children(func() {
						for i := 0; i < limit; i++ {
							p := matchedFiles[i]
							t.viewProjectFileRow(c, "search-file", p, p, 0, statusMap[p])
						}
						if len(matchedFiles) > limit {
							ui.Textf(c, "  ... 还有 %d 个匹配文件", len(matchedFiles)-limit).
								FontSize(11).
								TextColor(tk.TextTertiary)
						}
					})
				}
			}

			totalCodeLines := 0
			for _, m := range t.CodeSearchMatches {
				totalCodeLines += len(m.Lines)
			}
			openCodeSec := t.TreeOpenBool("search:sec:code")
			t.viewFileGroupHeader(c, "代码内容匹配", totalCodeLines, openCodeSec, func() {})
			if *openCodeSec {
				if t.CodeSearching && len(t.CodeSearchMatches) == 0 {
					ui.Text(c, "  正在搜索代码内容...").FontSize(11).TextColor(tk.TextTertiary)
					return
				}
				if len(t.CodeSearchMatches) == 0 {
					ui.Text(c, "  没有找到包含该内容的代码行").FontSize(11).TextColor(tk.TextTertiary)
					return
				}
				ui.Column(c).Gap(4).Children(func() {
					for _, fm := range t.CodeSearchMatches {
						fileMatch := fm
						fileOpen := t.TreeOpenBool("search:codefile:" + fileMatch.Path)

						// 文件分组头
						hdr := ui.Row(c.Key("codehdr:"+fileMatch.Path)).
							Height(24).
							Padding(0, 6).
							Gap(5).
							Radius(5).
							AlignItems(ui.Center).
							Justify(ui.SpaceBetween).
							Cursor(ui.CursorPointer)
						if hdr.Hovered() {
							hdr.Background(tk.FillTertiary)
						}
						if hdr.Clicked() {
							*fileOpen = !*fileOpen
						}
						hdr.Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Grow(1).ClipX().Children(func() {
								chev := lucide.ChevronRight()
								if *fileOpen {
									chev = lucide.ChevronDown()
								}
								display.Icon(c, chev, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextTertiary)
								display.Icon(c, lucide.FileCode(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
								ui.Text(c, fileMatch.Path).FontSize(12).Bold().SingleLine()
							})
							ui.Badge(c, fmt.Sprintf("%d", len(fileMatch.Lines)))
						})

						if *fileOpen {
							ui.Column(c).Gap(1).Children(func() {
								for _, ln := range fileMatch.Lines {
									lineItem := ln
									lineKey := fmt.Sprintf("codeline:%s:%d", fileMatch.Path, lineItem.Line)
									row := ui.Row(c.Key(lineKey)).
										Height(22).
										Padding(0, 6).
										PaddingStart(20).
										Gap(6).
										Radius(4).
										AlignItems(ui.Center).
										Cursor(ui.CursorPointer)
									if row.Hovered() {
										row.Background(tk.FillTertiary)
									}
									if row.Clicked() {
										t.OpenProjectFile(fileMatch.Path, lineItem.Line, q)
									}
									row.Children(func() {
										ui.Textf(c, "%d:", lineItem.Line).
											FontSize(11).
											Bold().
											TextColor(tk.TextTertiary).
											SingleLine().
											Width(28)
										renderCodeMatchRichLine(c, lineItem.Content, q, t.SearchCaseSensitive)
									})
								}
							})
						}
					}
				})
			}
		})
	})
}

// viewProjectTreeNode 递归渲染【项目文件】模式下的目录树节点（固定 24 DIP 行高，与文件行高度完全一致）。
// 输入参数 c 为 *ui.Context，scope 为键前缀，node 为当前树节点，depth 为缩进深度；无返回值。
func (t *RepoTab) viewProjectTreeNode(c *ui.Context, scope string, node *TreeNode, depth int) {
	tk := theme.TokensOf(c)
	if !node.IsDir {
		t.viewProjectFileRow(c, scope, node.FullPath, node.Name, depth, node.File.Status)
		return
	}

	openPtr := t.TreeOpenBool(scope + ":dir:" + node.FullPath)
	dirRow := ui.Row(c.Key(scope+":dirrow:"+node.FullPath)).
		Height(24).
		Padding(0, 6).
		PaddingStart(float32(6 + depth*12)).
		Gap(5).
		Radius(5).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if dirRow.Hovered() {
		dirRow.Background(tk.FillTertiary)
	}
	if dirRow.Clicked() {
		*openPtr = !*openPtr
	}
	dirRow.Children(func() {
		chev := lucide.ChevronRight()
		folderIc := lucide.Folder()
		if *openPtr {
			chev = lucide.ChevronDown()
			folderIc = lucide.FolderOpen()
		}
		display.Icon(c, chev, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextTertiary)
		display.Icon(c, folderIc, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
		ui.Text(c, node.Name).FontSize(12).Bold().SingleLine()
	})

	if *openPtr {
		for _, child := range node.Children {
			t.viewProjectTreeNode(c, scope, child, depth+1)
		}
	}
}

// viewProjectFileRow 渲染【项目文件】模式下的单个文件条目行（带有固定 24 DIP 行高、状态小圆点与单行文件名）。
// 输入参数 c 为 *ui.Context，scope 为键前缀，relPath 为文件相对路径，displayName 为显示名，depth 为缩进深度，status 为变更状态码；无返回值。
func (t *RepoTab) viewProjectFileRow(c *ui.Context, scope string, relPath string, displayName string, depth int, status git.FileStatusCode) {
	tk := theme.TokensOf(c)
	isSelected := t.SelectedProjectPath == relPath

	row := ui.Row(c.Key(scope+":projfile:"+relPath)).
		Height(24).
		Padding(0, 6).
		PaddingStart(float32(8 + depth*12)).
		Gap(6).
		Radius(5).
		AlignItems(ui.Center).
		Justify(ui.SpaceBetween).
		Cursor(ui.CursorPointer)

	if isSelected {
		row.Background(tk.FillSecondary)
	} else if row.Hovered() {
		row.Background(tk.FillTertiary)
	}
	if row.Clicked() {
		t.OpenProjectFile(relPath, 0, strings.TrimSpace(t.ProjectSearchQuery))
	}

	row.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).ClipX().Children(func() {
			if status != "" {
				renderStatusDot(c, status)
			} else {
				display.Icon(c, lucide.FileText(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextTertiary)
			}
			label := displayName
			if isSelected && t.IsProjectFileEdited() {
				label = "● " + label
			}
			ui.Text(c, label).FontSize(12).SingleLine().Grow(1)
		})
	})
}

// viewFileGroupHeader 渲染紧凑的单行分组折叠标题栏与组级图标按钮（使用原生 ui.Badge 并与左侧标题居中对齐）。
// 输入参数 c 为 *ui.Context，title 为标题，count 为数量，open 为折叠指针，actions 为右侧快捷图标回调；无返回值。
func (t *RepoTab) viewFileGroupHeader(c *ui.Context, title string, count int, open *bool, actions func()) {
	tk := theme.TokensOf(c)
	ui.Row(c).
		Height(24).
		Padding(0, 4).
		AlignItems(ui.Center).
		Justify(ui.SpaceBetween).
		Children(func() {
			toggleRow := ui.Row(c).Gap(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if toggleRow.Clicked() {
				*open = !*open
			}
			toggleRow.Children(func() {
				chev := lucide.ChevronRight()
				if *open {
					chev = lucide.ChevronDown()
				}
				display.Icon(c, chev, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextTertiary)
				ui.Text(c, title).FontSize(12).Bold().TextColor(tk.TextSecondary)
				ui.Badge(c, fmt.Sprintf("%d", count))
			})
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(actions)
		})
}

// viewFileCollection 根据当前用户选择的“平铺列表”或“目录树”模式渲染一组 ChangedFile。
// 输入参数 c 为 *ui.Context，scope 为唯一键作用域前缀，files 为文件切片，allowActions 表示是否显示暂存/放弃图标；无返回值。
func (t *RepoTab) viewFileCollection(c *ui.Context, scope string, files []git.ChangedFile, allowActions bool) {
	if !t.App.Settings.TreeMode {
		ui.Column(c).Gap(1).Children(func() {
			for _, f := range files {
				t.viewFileRow(c, scope, f, f.Path, 0, allowActions)
			}
		})
		return
	}

	nodes := BuildFileTree(t.Client.RepoName(), files, "")
	ui.Column(c).Gap(1).Children(func() {
		for _, n := range nodes {
			t.viewTreeNode(c, scope, n, 0, allowActions)
		}
	})
}

// viewTreeNode 递归渲染变更目录树节点（固定 24 DIP 行高，与下方文件行高度完全一致）。
// 输入参数 c 为 *ui.Context，scope 为作用域，node 为当前树节点，depth 为缩进深度，allowActions 为是否展示文件操作按钮；无返回值。
func (t *RepoTab) viewTreeNode(c *ui.Context, scope string, node *TreeNode, depth int, allowActions bool) {
	tk := theme.TokensOf(c)
	if !node.IsDir {
		t.viewFileRow(c, scope, node.File, node.Name, depth, allowActions)
		return
	}

	openPtr := t.TreeOpenBool(scope + ":dir:" + node.FullPath)
	dirRow := ui.Row(c.Key(scope+":dirrow:"+node.FullPath)).
		Height(24).
		Padding(0, 6).
		PaddingStart(float32(6 + depth*12)).
		Gap(5).
		Radius(5).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	if dirRow.Hovered() {
		dirRow.Background(tk.FillTertiary)
	}
	if dirRow.Clicked() {
		*openPtr = !*openPtr
	}
	dirRow.Children(func() {
		chev := lucide.ChevronRight()
		folderIc := lucide.Folder()
		if *openPtr {
			chev = lucide.ChevronDown()
			folderIc = lucide.FolderOpen()
		}
		display.Icon(c, chev, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextTertiary)
		display.Icon(c, folderIc, display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
		ui.Text(c, node.Name).FontSize(12).Bold().SingleLine()
	})

	if *openPtr {
		for _, child := range node.Children {
			t.viewTreeNode(c, scope, child, depth+1, allowActions)
		}
	}
}

// viewFileRow 渲染单个变更文件条目行：固定 24 DIP 行高（与目录行一致且防止 Hover 出现图标时高度变化），使用彩色圆点与 20x20 紧凑图标按钮。
// 输入参数 c 为 *ui.Context，scope 为键前缀，f 为文件结构体，displayName 为展示名称，depth 为缩进层级，allowActions 为是否允许操作；无返回值。
func (t *RepoTab) viewFileRow(c *ui.Context, scope string, f git.ChangedFile, displayName string, depth int, allowActions bool) {
	tk := theme.TokensOf(c)
	fileKey := f.Key()
	isSelected := t.SelectedFileKey == fileKey

	row := ui.Row(c.Key(scope+":file:"+fileKey)).
		Height(24).
		Padding(0, 6).
		PaddingStart(float32(8 + depth*12)).
		Gap(6).
		Radius(5).
		AlignItems(ui.Center).
		Justify(ui.SpaceBetween).
		Cursor(ui.CursorPointer)

	isHovered := row.Hovered()
	if isSelected {
		row.Background(tk.FillSecondary)
	} else if isHovered {
		row.Background(tk.FillTertiary)
	}

	if row.Clicked() {
		t.SelectFile(f)
	}

	row.Children(func() {
		// 左侧：彩色状态圆点 + 单行不换行文件名
		ui.Row(c).Gap(7).AlignItems(ui.Center).Grow(1).ClipX().Children(func() {
			renderStatusDot(c, f.Status)

			label := displayName
			if f.OrigPath != "" && f.OrigPath != f.Path {
				label = fmt.Sprintf("%s ← %s", displayName, f.OrigPath)
			}
			if entry, ok := t.DiffEntries[t.CurrentEntryKey(f)]; ok && entry.IsEdited() {
				label = "● " + label
			}
			ui.Text(c, label).FontSize(12).SingleLine().Grow(1)
		})

		// 右侧：紧凑的 Lobe ActionIcon 图标按钮（悬停或选中该行时显示，界面极其干净）
		if allowActions && (isHovered || isSelected) {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				if f.Staged {
					if action.ActionIcon(c, lucide.Minus(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 13,
						BlockSize: 20,
						Title:     "撤回暂存",
					}).Clicked() {
						fileCopy := f
						t.RunAsyncTask("取消暂存 "+fileCopy.Path, func() (string, error) {
							if err := t.Client.UnstageFile(fileCopy); err != nil {
								return "", err
							}
							return "已撤回暂存 " + fileCopy.Path, nil
						})
					}
				} else {
					if action.ActionIcon(c, lucide.RotateCcw(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 12,
						BlockSize: 20,
						Danger:    true,
						Title:     "放弃修改",
					}).Clicked() {
						fileCopy := f
						t.RunAsyncTask("放弃修改 "+fileCopy.Path, func() (string, error) {
							if err := t.Client.DiscardFile(fileCopy); err != nil {
								return "", err
							}
							return "已放弃 " + fileCopy.Path + " 的修改", nil
						})
					}
					if action.ActionIcon(c, lucide.Plus(), action.ActionIconProps{
						Variant:   action.ActionIconBorderless,
						GlyphSize: 13,
						BlockSize: 20,
						Title:     "暂存文件",
					}).Clicked() {
						fileCopy := f
						t.RunAsyncTask("暂存 "+fileCopy.Path, func() (string, error) {
							if err := t.Client.StageFile(fileCopy); err != nil {
								return "", err
							}
							return "已暂存 " + fileCopy.Path, nil
						})
					}
				}
			})
		}
	})
}

// viewCommitHistoryPanel 渲染“提交历史”模式下的上下分栏：上方为近期提交记录列表，下方为所选提交修改的文件列表。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewCommitHistoryPanel(c *ui.Context) {
	tk := theme.TokensOf(c)

	ui.SplitVertical(c, &t.CommitSplitHeight, func() {
		// 上半部：近期提交记录列表
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(6, 6).Gap(3).Children(func() {
				if len(t.Commits) == 0 {
					ui.Text(c, "当前仓库暂无提交记录").FontSize(11).TextColor(tk.TextTertiary)
					return
				}
				for _, cm := range t.Commits {
					commit := cm
					isSelected := commit.Hash == t.SelectedCommitHash
					item := ui.Column(c.Key("commit:"+commit.Hash)).
						Padding(5, 8).
						Gap(2).
						Radius(6).
						Cursor(ui.CursorPointer)
					if isSelected {
						item.Background(tk.FillSecondary)
					} else if item.Hovered() {
						item.Background(tk.FillTertiary)
					}
					if item.Clicked() {
						t.SelectCommit(commit.Hash)
					}
					item.Children(func() {
						ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
							display.Icon(c, lucide.GitCommitHorizontal(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
							ui.Text(c, commit.Subject).FontSize(12).Bold().Grow(1)
						})
						ui.Row(c).Justify(ui.SpaceBetween).Children(func() {
							ui.Text(c, commit.ShortHash+" · "+commit.Author).FontSize(11).TextColor(tk.TextTertiary)
							ui.Text(c, commit.RelTime).FontSize(11).TextColor(tk.TextTertiary)
						})
					})
				}
			})
		})
	}, func() {
		// 下半部：该次提交涉及的变更文件列表
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Height(28).Padding(0, 8).BorderWidth(1, 0, 1, 0).BorderColor(tk.BorderSecondary).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					short := t.SelectedCommitHash
					if len(short) > 7 {
						short = short[:7]
					}
					ui.Textf(c, "提交 %s 修改的文件", short).FontSize(12).Bold().TextColor(tk.TextSecondary).SingleLine()
					ui.Badge(c, fmt.Sprintf("%d", len(t.CommitFiles)))
				})
				t.viewTreeToggleIcon(c)
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(6, 6).Children(func() {
					filtered := FilterFiles(t.CommitFiles, t.SearchQuery)
					if len(filtered) == 0 {
						ui.Text(c, "无匹配的变更文件").FontSize(11).TextColor(tk.TextTertiary)
						return
					}
					t.viewFileCollection(c, "commitfiles:"+t.SelectedCommitHash, filtered, false)
				})
			})
		})
	}).SplitReset(240).Grow(1)
}

// renderCodeMatchRichLine 渲染类似 VSCode 全局搜索结果的单行代码预览：自动省略匹配词前后的冗余内容，并对匹配关键字做高亮底色与加粗。
// 输入参数 c 为 *ui.Context，line 为原始代码行，query 为搜索词，caseSensitive 表示是否区分大小写；无返回值。
func renderCodeMatchRichLine(c *ui.Context, line string, query string, caseSensitive bool) {
	tk := theme.TokensOf(c)
	prefix, matched, suffix := extractCenteredSnippet(line, query, caseSensitive)

	if matched == "" {
		ui.Text(c, line).
			FontSize(12).
			TextColor(tk.TextSecondary).
			SingleLine().
			Grow(1)
		return
	}

	var spans []ui.Span
	if prefix != "" {
		spans = append(spans, ui.Span{
			Text:  prefix,
			Size:  12,
			Color: tk.TextSecondary,
		})
	}
	spans = append(spans, ui.Span{
		Text:       matched,
		Size:       12,
		Weight:     600,
		Color:      tk.Text,
		Background: tk.WarningFillSecondary,
		Radius:     3,
		PaddingX:   2,
	})
	if suffix != "" {
		spans = append(spans, ui.Span{
			Text:  suffix,
			Size:  12,
			Color: tk.TextSecondary,
		})
	}

	ui.RichText(c, spans...).
		FontSize(12).
		SingleLine().
		Grow(1)
}

// extractCenteredSnippet 在代码行中查找匹配词，按 rune 截取匹配位置前最多 16 个字符与后最多 40 个字符，超出部分以 "..." 省略。
// 输入参数 line 为原始代码行，query 为搜索词，caseSensitive 为是否区分大小写；返回前缀、匹配原词和后缀三段字符串。
func extractCenteredSnippet(line string, query string, caseSensitive bool) (prefix, matched, suffix string) {
	cleanLine := strings.TrimSpace(strings.ReplaceAll(line, "\t", " "))
	q := strings.TrimSpace(query)
	if q == "" {
		return cleanLine, "", ""
	}

	haystack := cleanLine
	needle := q
	if !caseSensitive {
		haystack = strings.ToLower(cleanLine)
		needle = strings.ToLower(q)
	}

	byteIdx := strings.Index(haystack, needle)
	if byteIdx < 0 {
		return cleanLine, "", ""
	}

	// 将字节索引安全地转换为 rune 切片索引，避免截断中文字符
	beforeRunes := []rune(cleanLine[:byteIdx])
	matchRunes := []rune(cleanLine[byteIdx:])
	qRuneLen := len([]rune(q))
	if qRuneLen > len(matchRunes) {
		qRuneLen = len(matchRunes)
	}
	matched = string(matchRunes[:qRuneLen])
	afterRunes := matchRunes[qRuneLen:]

	const maxPre = 16
	const maxPost = 45

	if len(beforeRunes) > maxPre {
		prefix = "..." + string(beforeRunes[len(beforeRunes)-maxPre:])
	} else {
		prefix = string(beforeRunes)
	}

	if len(afterRunes) > maxPost {
		suffix = string(afterRunes[:maxPost]) + "..."
	} else {
		suffix = string(afterRunes)
	}
	return prefix, matched, suffix
}

// renderCompactCheckbox 使用原生 ui.CheckboxBase 渲染一个方框与文字严格垂直居中、间距为 5 DIP 的紧凑复选框。
// 输入参数 c 为 *ui.Context，checked 为绑定的布尔指针，label 为文字；返回该复选框在本帧是否发生了改变。
func renderCompactCheckbox(c *ui.Context, checked *bool, label string) bool {
	tk := theme.TokensOf(c)
	cb := ui.CheckboxBase(c, checked).
		Height(22).
		Gap(5).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer)

	hovered := cb.Hovered()
	cb.Children(func() {
		box := ui.Box(c).
			Size(14, 14).
			Radius(3).
			Center()
		if *checked {
			box.Background(tk.Blue).Border(1, tk.Blue)
			box.Children(func() {
				ui.Icon(c, lucide.Check()).Size(10, 10).TextColor(tk.White)
			})
		} else {
			borderCol := tk.Border
			if hovered {
				borderCol = tk.Blue
			}
			box.Background(tk.BgContainer).Border(1, borderCol)
		}
		ui.Text(c, label).
			FontSize(12).
			LineHeight(1).
			TextColor(tk.TextSecondary).
			SingleLine()
	})
	return cb.Changed()
}

// renderCompactPrimaryButton 使用原生 ui.ButtonBase 渲染轻量柔和色调的提交按钮（高度 22 DIP、告别厚重纯黑背景，与左侧复选框严格居中对齐）。
// 输入参数 c 为 *ui.Context，iconSvg 为前置小图标（可为 nil），label 为文字，disabled 为是否禁用；返回按钮 ui.Element。
func renderCompactPrimaryButton(c *ui.Context, iconSvg *ui.SVG, label string, disabled bool) ui.Element {
	tk := theme.TokensOf(c)
	btn := ui.ButtonBase(c).
		Height(22).
		Padding(0, 8).
		Gap(4).
		Radius(5).
		AlignItems(ui.Center).
		Justify(ui.Center).
		Disabled(disabled).
		DimDisabled(false)

	bg := tk.BlueFillTertiary
	borderCol := tk.Blue
	fg := tk.Blue
	if disabled {
		bg = tk.FillQuaternary
		borderCol = tk.BorderSecondary
		fg = tk.TextDisabled
	} else {
		btn.Cursor(ui.CursorPointer)
		if btn.Hovered() {
			bg = tk.Blue
			fg = tk.White
		}
	}
	btn.Background(bg).Border(1, borderCol)

	btn.Children(func() {
		if iconSvg != nil {
			ui.Icon(c, iconSvg).Size(11, 11).TextColor(fg)
		}
		ui.Text(c, label).
			FontSize(12).
			LineHeight(1).
			FontWeight(500).
			TextColor(fg).
			SingleLine()
	})
	return btn
}

// renderStatusDot 绘制一个 8x8 的彩色圆点来表示文件变更状态（新文件/新增为绿色圆点，修改为橙黄圆点，删除为红色圆点，重命名为蓝色圆点）。
// 输入参数 c 为 *ui.Context，code 为变更状态码；无返回值。
func renderStatusDot(c *ui.Context, code git.FileStatusCode) {
	tk := theme.TokensOf(c)
	color := statusDotColor(tk, code)
	ui.Box(c).Size(8, 8).Radius(4).Background(color)
}

// statusDotColor 根据 Lobe UI 颜色 Token 返回文件状态圆点的颜色。
// 输入参数 tk 为 Lobe 主题 Tokens，code 为变更状态码；返回 ui.Color。
func statusDotColor(tk *theme.Tokens, code git.FileStatusCode) ui.Color {
	switch code {
	case git.StatusAdded, git.StatusUntracked:
		return tk.Success
	case git.StatusDeleted, git.StatusConflict:
		return tk.Error
	case git.StatusRenamed:
		return tk.Blue
	default:
		return tk.Warning
	}
}

// statusChineseLabel 将 Git 状态码映射为中文说明（用于 Tooltip 或详情提示）。
// 输入参数 code 为 FileStatusCode；返回中文简称字符串。
func statusChineseLabel(code git.FileStatusCode) string {
	switch code {
	case git.StatusAdded:
		return "新增"
	case git.StatusUntracked:
		return "新文件"
	case git.StatusDeleted:
		return "删除"
	case git.StatusRenamed:
		return "重命名"
	case git.StatusConflict:
		return "冲突"
	default:
		return "修改"
	}
}
