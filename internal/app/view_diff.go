package app

import (
	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// viewDiffPane 渲染右侧主区域：在【项目文件】模式下渲染带 Git 变更标识栏的全功能代码查看/编辑器；
// 在【代码变更】和【提交历史】模式下渲染单文件/一页流代码差异对比视图（DiffView）。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewDiffPane(c *ui.Context) {
	if SidebarMode(t.SidebarMode) == ModeProjectFiles {
		t.viewProjectFileEditorPane(c)
		return
	}

	tk := theme.TokensOf(c)

	ui.Column(c).Fill().Background(tk.BgContainer).Children(func() {
		// 1. 顶部 Diff 控制栏
		t.viewDiffToolbar(c)

		// 2. Diff 主体内容区
		files := t.ActiveFileList()
		if len(files) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{
					Icon:        lucide.FileDiff(),
					Title:       "当前没有待审查的代码修改",
					Description: "当你在目录中修改文件后，此处将自动实时展示代码差异；也可点击左侧「项目文件」浏览或搜索全部代码",
				})
			})
			return
		}

		if t.App.Settings.AllChanges {
			t.viewAllChangesStream(c, files)
		} else {
			t.viewSingleFileDiff(c, files)
		}
	})
}

// viewPaneHeaderBar 统一渲染右侧面板（【项目文件】、【代码变更】、【提交历史】）的顶部标题工具栏，固定高度 36 DIP，确保所有模式下标题栏高度与对齐方式完全一致。
// 输入参数 c 为 *ui.Context，leftContent 为左侧文件路径与状态渲染回调，rightActions 为右侧操作按钮渲染回调；无返回值。
func (t *RepoTab) viewPaneHeaderBar(c *ui.Context, leftContent func(), rightActions func()) {
	tk := theme.TokensOf(c)
	ui.Row(c).
		Height(36).
		Padding(0, 12).
		Gap(8).
		AlignItems(ui.Center).
		Background(tk.BgLayout).
		BorderWidth(0, 0, 1, 0).
		BorderColor(tk.BorderSecondary).
		Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Grow(1).Shrink(1).ClipX().Children(leftContent)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(rightActions)
		})
}

// viewProjectFileEditorPane 渲染【项目文件】模式下的右侧代码查看与编辑面板（复用统一的 viewPaneHeaderBar）。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewProjectFileEditorPane(c *ui.Context) {
	tk := theme.TokensOf(c)
	statusMap := t.WorkingStatusMap()
	fileStatus := statusMap[t.SelectedProjectPath]

	ui.Column(c).Fill().Background(tk.BgContainer).Children(func() {
		// 统一高度的顶部工具栏
		t.viewPaneHeaderBar(c, func() {
			if t.SelectedProjectPath != "" {
				if fileStatus != "" {
					renderStatusDot(c, fileStatus)
				} else {
					display.Icon(c, lucide.FileCode(), display.IconProps{Size: display.IconSmall}).TextColor(tk.TextSecondary)
				}
				ui.Text(c, t.SelectedProjectPath).FontSize(13).Bold().SingleLine()
			} else {
				ui.Text(c, "请在左侧选择文件或搜索代码").FontSize(12).TextColor(tk.TextTertiary).SingleLine()
			}
		}, func() {
			if t.IsProjectFileEdited() {
				if action.ActionIcon(c, lucide.Undo2(), action.ActionIconProps{
					Variant: action.ActionIconOutlined,
					Size:    theme.Small,
					Title:   "放弃未保存修改",
				}).Clicked() {
					t.ProjectFileEditor.Buffer.Set(t.ProjectFileBaseText)
				}
				if action.Button(c, "保存 (Cmd+S)", action.ButtonProps{
					Variant: action.Primary,
					Size:    theme.Small,
				}).Clicked() {
					t.SaveCurrentProjectFile()
				}
			}

			if fileStatus != "" {
				if action.Button(c, "查看变更对比", action.ButtonProps{
					Variant: action.Default,
					Size:    theme.Small,
				}).Clicked() {
					t.SwitchToFileDiff(t.SelectedProjectPath)
				}
			}
		})

		// 主体编辑器区
		if t.SelectedProjectPath == "" {
			ui.Column(c).Grow(1).Center().Children(func() {
				display.Empty(c, display.EmptyProps{
					Icon:        lucide.FileSearch(),
					Title:       "未选择项目文件",
					Description: "在左侧点击任意文件，或输入关键字搜索代码内容即可在此处查看与编辑",
				})
			})
			return
		}

		if t.ProjectFileLoading {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				display.Icon(c, lucide.RefreshCw(), display.IconProps{Size: display.IconMiddle, Spin: true}).TextColor(tk.Primary)
				ui.Text(c, "正在读取文件内容...").FontSize(12).TextColor(tk.TextTertiary)
			})
			return
		}

		if t.ProjectFileIsBinary {
			ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
				ui.Text(c, "二进制文件不支持作为代码文本打开").FontSize(13).Bold()
				ui.Text(c, t.SelectedProjectPath).FontSize(12).TextColor(tk.TextTertiary)
			})
			return
		}

		if t.ProjectFileEditor != nil {
			editor.View(c.Key("proj-editor:"+t.SelectedProjectPath), t.ProjectFileEditor).Grow(1)
		}
	})
}

// viewDiffToolbar 渲染右侧【代码变更】与【提交历史】的顶部操作栏（复用 viewPaneHeaderBar，向【项目文件】样式对齐，不使用撑高行高的 Lobe Tag）。
// 输入参数 c 为 *ui.Context；无返回值。
func (t *RepoTab) viewDiffToolbar(c *ui.Context) {
	tk := theme.TokensOf(c)

	var curFile *git.ChangedFile
	files := t.ActiveFileList()
	for i := range files {
		if files[i].Key() == t.SelectedFileKey {
			curFile = &files[i]
			break
		}
	}
	if curFile == nil && len(files) > 0 {
		curFile = &files[0]
		t.SelectedFileKey = curFile.Key()
	}

	var curEntry *FileDiffEntry
	if curFile != nil {
		curEntry = t.DiffEntries[t.CurrentEntryKey(*curFile)]
	}

	t.viewPaneHeaderBar(c, func() {
		if t.App.Settings.AllChanges {
			ui.Textf(c, "全部变更一页流 (%d 个文件)", len(files)).FontSize(13).Bold().SingleLine()
		} else if curFile != nil {
			renderStatusDot(c, curFile.Status)
			if SidebarMode(t.SidebarMode) == ModeCommitHistory {
				short := t.SelectedCommitHash
				if len(short) > 7 {
					short = short[:7]
				}
				ui.Badge(c, short)
			} else if curFile.Staged {
				ui.Badge(c, "已暂存")
			}
			ui.Text(c, curFile.Path).FontSize(13).Bold().SingleLine()

			if curEntry != nil && curEntry.Diff != nil && !curEntry.IsBinary {
				added, deleted := curEntry.Diff.Stats()
				renderDiffStatText(c, added, deleted)
			}
		} else {
			ui.Text(c, "未选择文件").FontSize(12).TextColor(tk.TextTertiary).SingleLine()
		}
	}, func() {
		// 如果用户正在微调当前文件，展示紧凑图标按钮与磁盘冲突提示
		if !t.App.Settings.AllChanges && curEntry != nil {
			if curEntry.DiskChanged {
				ui.Text(c, "⚠ 磁盘文件已更新").FontSize(12).TextColor(tk.Warning).SingleLine()
				if action.ActionIcon(c, lucide.RefreshCw(), action.ActionIconProps{
					Variant: action.ActionIconOutlined,
					Size:    theme.Small,
					Title:   "重新加载磁盘最新版本",
				}).Clicked() {
					t.loadSingleDiffAsync(curEntry.File, true)
				}
			}
			if curEntry.IsEdited() {
				if action.ActionIcon(c, lucide.Undo2(), action.ActionIconProps{
					Variant: action.ActionIconOutlined,
					Size:    theme.Small,
					Title:   "放弃微调",
				}).Clicked() {
					curEntry.Diff.New.Set(curEntry.BaseNewText)
					curEntry.DiskChanged = false
				}
				if action.Button(c, "保存 (Cmd+S)", action.ButtonProps{
					Variant: action.Primary,
					Size:    theme.Small,
				}).Clicked() {
					t.SaveEditedFile(curEntry)
				}
			}
		}

		// 单文件模式下提供上一文件 / 下一文件紧凑图标按钮
		if !t.App.Settings.AllChanges && len(files) > 1 {
			if action.ActionIcon(c, lucide.ArrowUp(), action.ActionIconProps{
				Variant: action.ActionIconOutlined,
				Size:    theme.Small,
				Title:   "上一个文件",
			}).Clicked() {
				t.SelectAdjacentFile(-1)
			}
			if action.ActionIcon(c, lucide.ArrowDown(), action.ActionIconProps{
				Variant: action.ActionIconOutlined,
				Size:    theme.Small,
				Title:   "下一个文件",
			}).Clicked() {
				t.SelectAdjacentFile(1)
			}
		}

		// 切换“单文件 / 全部一页流”（原生紧凑胶囊组，高度 24 DIP）
		streamModeIdx := 0
		if t.App.Settings.AllChanges {
			streamModeIdx = 1
		}
		if renderCompactSegmented(c, &streamModeIdx, "单文件", "全部一页流") {
			t.App.Settings.AllChanges = (streamModeIdx == 1)
			t.App.SaveSettings()
		}

		// 切换“左右分栏 / 上下合并”（原生紧凑胶囊组，高度 24 DIP）
		layoutIdx := 0
		if t.App.Settings.UnifiedDiff {
			layoutIdx = 1
		}
		if renderCompactSegmented(c, &layoutIdx, "左右分栏", "上下合并") {
			t.App.Settings.UnifiedDiff = (layoutIdx == 1)
			t.App.ApplyDiffLayoutToAllTabs()
		}
	})
}

// renderDiffStatText 使用原生文本渲染紧凑的 "+N" 和 "-M" 行数统计，不撑高标题栏。
// 输入参数 c 为 *ui.Context，added 为新增行数，deleted 为删除行数；无返回值。
func renderDiffStatText(c *ui.Context, added, deleted int) {
	tk := theme.TokensOf(c)
	if added > 0 || deleted == 0 {
		ui.Textf(c, "+%d", added).FontSize(12).Bold().TextColor(tk.Success).SingleLine()
	}
	if deleted > 0 {
		ui.Textf(c, "-%d", deleted).FontSize(12).Bold().TextColor(tk.Error).SingleLine()
	}
}

// renderCompactSegmented 使用原生布局渲染固定高度 24 DIP 的紧凑分段切换器，保证与 24 DIP 图标按钮完美对齐。
// 输入参数 c 为 *ui.Context，selected 为当前选中索引指针，labels 为分段标签；返回本帧是否发生切换。
func renderCompactSegmented(c *ui.Context, selected *int, labels ...string) bool {
	tk := theme.TokensOf(c)
	changed := false
	ui.Row(c).
		Height(24).
		Padding(2).
		Gap(2).
		Radius(6).
		AlignItems(ui.Center).
		Background(tk.FillTertiary).
		Border(1, tk.BorderSecondary).
		Children(func() {
			for i, lb := range labels {
				idx := i
				isActive := *selected == idx
				item := ui.Row(c).
					Height(20).
					Padding(0, 8).
					Radius(4).
					Center().
					Cursor(ui.CursorPointer)
				if isActive {
					item.Background(tk.BgContainer)
				} else if item.Hovered() {
					item.Background(tk.FillQuaternary)
				}
				if item.Clicked() && *selected != idx {
					*selected = idx
					changed = true
				}
				textColor := tk.TextSecondary
				if isActive {
					textColor = tk.Text
				}
				item.Children(func() {
					lbl := ui.Text(c, lb).FontSize(12).TextColor(textColor).SingleLine()
					if isActive {
						lbl.Bold()
					}
				})
			}
		})
	return changed
}

// viewSingleFileDiff 渲染单个选中文件的全屏 DiffView。
// 输入参数 c 为 *ui.Context，files 为当前激活的文件列表；无返回值。
func (t *RepoTab) viewSingleFileDiff(c *ui.Context, files []git.ChangedFile) {
	tk := theme.TokensOf(c)
	var selected *git.ChangedFile
	for i := range files {
		if files[i].Key() == t.SelectedFileKey {
			selected = &files[i]
			break
		}
	}
	if selected == nil {
		selected = &files[0]
		t.SelectedFileKey = selected.Key()
	}

	entryKey := t.CurrentEntryKey(*selected)
	entry, ok := t.DiffEntries[entryKey]
	if !ok {
		t.loadSingleDiffAsync(*selected, false)
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			display.Icon(c, lucide.RefreshCw(), display.IconProps{Size: display.IconMiddle, Spin: true}).TextColor(tk.Primary)
			ui.Text(c, "正在加载代码差异对比...").FontSize(12).TextColor(tk.TextTertiary)
		})
		return
	}

	if entry.IsBinary {
		ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
			ui.Text(c, "二进制文件已修改，不支持文本对比预览").FontSize(13).Bold()
			ui.Text(c, selected.Path).FontSize(12).TextColor(tk.TextTertiary)
		})
		return
	}

	entry.Diff.FitContent = false
	entry.syncLayoutFromSettings(t.App.Settings.UnifiedDiff)
	editor.DiffView(c.Key("single-diff:"+entryKey), entry.Diff).Grow(1)
}

// viewAllChangesStream 在一个连续的垂直滚动容器中渲染当前所有变更文件的 Diff 卡片（全部变更一页流模式，卡片头部全面采用彩色状态圆点与 Lobe 图标按钮）。
// 输入参数 c 为 *ui.Context，files 为当前激活的文件列表；无返回值。
func (t *RepoTab) viewAllChangesStream(c *ui.Context, files []git.ChangedFile) {
	tk := theme.TokensOf(c)

	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(10).Gap(12).Children(func() {
			for _, f := range files {
				fileCopy := f
				entryKey := t.CurrentEntryKey(fileCopy)
				entry, ok := t.DiffEntries[entryKey]
				if !ok {
					t.loadSingleDiffAsync(fileCopy, false)
				}

				ui.Column(c.Key("stream-card:"+entryKey)).
					Radius(8).
					Border(1, tk.BorderSecondary).
					Background(tk.BgContainer).
					Clip().
					Children(func() {
						// 卡片头部：彩色圆点、文件路径、+N/-M Tag 以及紧凑 Lobe ActionIcon 操作图标
						ui.Row(c).
							Padding(6, 12).
							Gap(8).
							AlignItems(ui.Center).
							Background(tk.BgLayout).
							BorderWidth(0, 0, 1, 0).
							BorderColor(tk.BorderSecondary).
							Children(func() {
								renderStatusDot(c, fileCopy.Status)
								if fileCopy.Staged && SidebarMode(t.SidebarMode) == ModeWorkingChanges {
									ui.Badge(c, "已暂存")
								}
								ui.Text(c, fileCopy.Path).FontSize(12).Bold().SingleLine().Grow(1)

								if entry != nil && entry.Diff != nil && !entry.IsBinary {
									added, deleted := entry.Diff.Stats()
									renderDiffStatText(c, added, deleted)
								}

								if entry != nil && entry.IsEdited() {
									if action.ActionIcon(c, lucide.Undo2(), action.ActionIconProps{
										Variant:   action.ActionIconOutlined,
										GlyphSize: 13,
										BlockSize: 24,
										Title:     "放弃微调",
									}).Clicked() {
										entry.Diff.New.Set(entry.BaseNewText)
									}
									if action.ActionIcon(c, lucide.Save(), action.ActionIconProps{
										Variant:   action.ActionIconFilled,
										GlyphSize: 13,
										BlockSize: 24,
										Active:    true,
										Title:     "保存微调",
									}).Clicked() {
										t.SaveEditedFile(entry)
									}
								}

								if SidebarMode(t.SidebarMode) == ModeWorkingChanges {
									if fileCopy.Staged {
										if action.ActionIcon(c, lucide.Minus(), action.ActionIconProps{
											Variant:   action.ActionIconOutlined,
											GlyphSize: 13,
											BlockSize: 24,
											Title:     "撤回暂存",
										}).Clicked() {
											t.RunAsyncTask("取消暂存 "+fileCopy.Path, func() (string, error) {
												if err := t.Client.UnstageFile(fileCopy); err != nil {
													return "", err
												}
												return "已撤回暂存 " + fileCopy.Path, nil
											})
										}
									} else {
										if action.ActionIcon(c, lucide.RotateCcw(), action.ActionIconProps{
											Variant:   action.ActionIconOutlined,
											GlyphSize: 13,
											BlockSize: 24,
											Danger:    true,
											Title:     "放弃修改",
										}).Clicked() {
											t.RunAsyncTask("放弃修改 "+fileCopy.Path, func() (string, error) {
												if err := t.Client.DiscardFile(fileCopy); err != nil {
													return "", err
												}
												return "已放弃 " + fileCopy.Path + " 的修改", nil
											})
										}
										if action.ActionIcon(c, lucide.Plus(), action.ActionIconProps{
											Variant:   action.ActionIconOutlined,
											GlyphSize: 13,
											BlockSize: 24,
											Title:     "暂存文件",
										}).Clicked() {
											t.RunAsyncTask("暂存 "+fileCopy.Path, func() (string, error) {
												if err := t.Client.StageFile(fileCopy); err != nil {
													return "", err
												}
												return "已暂存 " + fileCopy.Path, nil
											})
										}
									}
								}
							})

						// 卡片主体：FitContent 模式的 DiffView
						if entry == nil {
							ui.Row(c).Padding(14).Gap(8).AlignItems(ui.Center).Children(func() {
								display.Icon(c, lucide.RefreshCw(), display.IconProps{Size: display.IconSmall, Spin: true}).TextColor(tk.Primary)
								ui.Text(c, "正在加载文件差异...").FontSize(12).TextColor(tk.TextTertiary)
							})
						} else if entry.IsBinary {
							ui.Row(c).Padding(14).Children(func() {
								ui.Text(c, "二进制文件已修改，不支持文本对比预览").FontSize(12).TextColor(tk.TextTertiary)
							})
						} else {
							entry.Diff.FitContent = true
							entry.syncLayoutFromSettings(t.App.Settings.UnifiedDiff)
							editor.DiffView(c.Key("stream-diff:"+entryKey), entry.Diff).WidthPercent(100)
						}
					})
			}
		})
	})
}
