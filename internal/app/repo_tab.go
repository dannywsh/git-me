package app

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"git-me/internal/git"
	"git-me/internal/watcher"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// SidebarMode 定义左侧边栏的当前视图模式。
type SidebarMode int

const (
	ModeWorkingChanges SidebarMode = 0 // 代码变更
	ModeCommitHistory  SidebarMode = 1 // 提交历史
	ModeProjectFiles   SidebarMode = 2 // 项目文件与代码搜索
)

// FileDiffEntry 维护单个变更文件对应的 editor.DiffState 及基线文本快照。
type FileDiffEntry struct {
	File        git.ChangedFile
	Diff        *editor.DiffState
	BaseOldText string
	BaseNewText string
	IsBinary    bool
	Editable    bool
	DiskChanged bool // 用户正在编辑该文件时磁盘发生了外部更新
}

// diffLoad 标识正在读取的 Diff 所属的状态快照及是否覆盖未保存编辑。
type diffLoad struct {
	statusSeq uint64
	force     bool
}

// IsEdited 检查用户是否在 DiffView 的新文本侧做了尚未保存到磁盘的微调编辑。
// 无输入参数，已编辑未保存返回 true。
func (e *FileDiffEntry) IsEdited() bool {
	if e == nil || !e.Editable || e.Diff == nil || e.Diff.New == nil {
		return false
	}
	return e.Diff.New.String() != e.BaseNewText
}

// RepoTab 封装单个打开的 Git 仓库标签页的全部 UI 状态与后台 Git 操作逻辑。
type RepoTab struct {
	App              *App
	Client           *git.Client
	Watcher          *watcher.RepoWatcher
	closed           bool
	refreshSeq       atomic.Uint64
	refreshForce     bool
	workingStatusSeq uint64
	lastRefreshAt    time.Time

	// 顶栏分支与同步状态
	BranchInfo     git.BranchInfo
	SelectedBranch string
	BranchMenuOpen bool
	RecentMenuOpen bool
	NewBranchModal bool
	NewBranchName  string
	BranchErrMsg   string
	Busy           bool
	StatusBanner   string
	IsBannerError  bool

	// 左侧栏通用状态
	SidebarMode      int // 0: 代码变更, 1: 提交历史, 2: 项目文件
	CommitMessage    string
	AmendCommit      bool
	SearchQuery      string
	StagedOpen       bool
	UnstagedOpen     bool
	StagedFiles      []git.ChangedFile
	UnstagedFiles    []git.ChangedFile
	SelectedFileKey  string
	TreeExpandStates map[string]*bool

	// 提交历史模式状态
	Commits            []git.CommitEntry
	SelectedCommitHash string
	CommitFiles        []git.ChangedFile
	CommitSplitHeight  float32

	// 项目文件与全文代码搜索状态
	ProjectFiles        []string
	ProjectSearchQuery  string
	SearchCaseSensitive bool
	SearchIncludeIgnore bool
	CodeSearchMatches   []git.FileSearchMatch
	CodeSearching       bool
	SelectedProjectPath string
	ProjectFileEditor   *editor.State
	ProjectFileBaseText string
	ProjectFileIsBinary bool
	ProjectFileLoading  bool
	searchSeq           atomic.Uint64

	// 右侧 Diff 状态（按文件 key 缓存，支持单文件与全部变更一页流）
	DiffEntries  map[string]*FileDiffEntry
	loadingDiffs map[string]diffLoad
}

// NewRepoTab 为指定的 Git Client 初始化一个仓库标签页，并启动后台刷新与文件监听。
// 输入参数 parent 为顶层 App 引用，client 为已校验的 Git 客户端；返回 *RepoTab 实例。
func NewRepoTab(parent *App, client *git.Client) *RepoTab {
	tab := &RepoTab{
		App:               parent,
		Client:            client,
		StagedOpen:        true,
		UnstagedOpen:      true,
		TreeExpandStates:  make(map[string]*bool),
		CommitSplitHeight: 240,
		DiffEntries:       make(map[string]*FileDiffEntry),
		loadingDiffs:      make(map[string]diffLoad),
	}

	tab.Watcher = watcher.NewRepoWatcher(client.RepoPath(), func() {
		tab.RefreshAsync(false)
	})
	tab.RefreshAsync(true)
	return tab
}

// Close 关闭当前仓库标签页的文件监听器并清理资源。
// 无输入参数，无返回值。
func (t *RepoTab) Close() {
	t.closed = true
	t.refreshSeq.Add(1)
	if t.Watcher != nil {
		t.Watcher.Stop()
		t.Watcher = nil
	}
}

// TreeOpenBool 获取或创建目录树某个路径节点的展开状态布尔指针（默认展开）。
// 输入参数 key 为目录路径键；返回持久化的 *bool 指针。
func (t *RepoTab) TreeOpenBool(key string) *bool {
	if b, ok := t.TreeExpandStates[key]; ok {
		return b
	}
	v := true
	t.TreeExpandStates[key] = &v
	return &v
}

// WorkingStatusMap 返回当前仓库所有未提交变更文件的路径到状态码映射，用于在项目文件树中标出已修改文件。
// 无输入参数，返回 map[string]git.FileStatusCode。
func (t *RepoTab) WorkingStatusMap() map[string]git.FileStatusCode {
	m := make(map[string]git.FileStatusCode, len(t.StagedFiles)+len(t.UnstagedFiles))
	for _, f := range t.StagedFiles {
		m[f.Path] = f.Status
	}
	for _, f := range t.UnstagedFiles {
		m[f.Path] = f.Status
	}
	return m
}

// IsProjectFileEdited 检查用户在【项目文件】代码编辑器中是否修改了文件内容且尚未保存。
// 无输入参数，有未保存修改返回 true。
func (t *RepoTab) IsProjectFileEdited() bool {
	if t.ProjectFileEditor == nil || t.ProjectFileEditor.Buffer == nil || t.ProjectFileIsBinary {
		return false
	}
	return t.ProjectFileEditor.Buffer.String() != t.ProjectFileBaseText
}

// OpenProjectFile 在右侧代码编辑器中打开指定路径的项目文件，并在 targetLine > 0 时自动跳转到对应行和关键字高亮。
// 输入参数 relPath 为相对于仓库根目录的路径，targetLine 为 1-based 目标行号（0 表示不跳转），highlightQuery 为要高亮的搜索词。
// 无返回值。
func (t *RepoTab) OpenProjectFile(relPath string, targetLine int, highlightQuery string) {
	// 如果当前已经是同一个文件且未在加载中，仅执行行号跳转和查找高亮
	if t.SelectedProjectPath == relPath && t.ProjectFileEditor != nil && !t.ProjectFileLoading {
		t.scrollEditorToLine(targetLine, highlightQuery)
		return
	}

	t.SelectedProjectPath = relPath
	t.ProjectFileLoading = true

	go func() {
		content, err := t.Client.ReadProjectFile(relPath)
		t.App.RunUI(func() {
			t.ProjectFileLoading = false
			if err != nil {
				t.StatusBanner = "读取文件失败: " + err.Error()
				t.IsBannerError = true
				return
			}
			t.ProjectFileIsBinary = content.IsBinary
			t.ProjectFileBaseText = content.Text
			if content.IsBinary {
				t.ProjectFileEditor = nil
				return
			}

			buf := editor.NewBuffer(content.Text)
			lang := editor.LanguageForText(relPath, content.Text)
			if lang == nil {
				lang = editor.LanguageFor(relPath)
			}
			if lang != nil {
				buf.SetLanguage(lang)
			}

			st := &editor.State{
				Buffer:   buf,
				ReadOnly: false,
			}
			if content.HasHead {
				baseBuf := editor.NewBuffer(content.HeadText)
				if lang != nil {
					baseBuf.SetLanguage(lang)
				}
				st.DiffBase = baseBuf
			}
			t.ProjectFileEditor = st
			t.scrollEditorToLine(targetLine, highlightQuery)
		})
	}()
}

// scrollEditorToLine 将右侧项目文件代码编辑器滚动并定位到指定的 1-based 行号，并标记搜索高亮词。
// 输入参数 targetLine 为 1-based 行号，highlightQuery 为搜索关键字；无返回值。
func (t *RepoTab) scrollEditorToLine(targetLine int, highlightQuery string) {
	if t.ProjectFileEditor == nil {
		return
	}
	if strings.TrimSpace(highlightQuery) != "" {
		t.ProjectFileEditor.Find(strings.TrimSpace(highlightQuery))
	}
	if targetLine > 0 {
		lineIdx := max(0, targetLine-1)
		t.ProjectFileEditor.GoTo(editor.Pos{Line: lineIdx, Col: 0})
		t.ProjectFileEditor.ScrollTo(lineIdx)
	}
}

// SaveCurrentProjectFile 将用户在【项目文件】编辑器中修改后的内容写回磁盘，并刷新 Git 状态。
// 无输入参数，无返回值。
func (t *RepoTab) SaveCurrentProjectFile() {
	if t.SelectedProjectPath == "" || t.ProjectFileEditor == nil || t.ProjectFileEditor.Buffer == nil {
		return
	}
	relPath := t.SelectedProjectPath
	newText := t.ProjectFileEditor.Buffer.String()
	go func() {
		if err := t.Client.SaveWorkingFile(relPath, newText); err != nil {
			t.setBanner("保存文件失败: "+err.Error(), true)
			return
		}
		t.App.RunUI(func() {
			t.ProjectFileBaseText = newText
			t.StatusBanner = fmt.Sprintf("已保存文件 %s", relPath)
			t.IsBannerError = false
		})
		t.RefreshAsync(true)
	}()
}

// SwitchToFileDiff 从【项目文件】编辑器一键跳转到【代码变更】模式并选中该文件的对比视图。
// 输入参数 relPath 为文件相对路径；无返回值。
func (t *RepoTab) SwitchToFileDiff(relPath string) {
	for _, f := range t.UnstagedFiles {
		if f.Path == relPath {
			t.SidebarMode = int(ModeWorkingChanges)
			t.App.Settings.AllChanges = false
			t.SelectFile(f)
			return
		}
	}
	for _, f := range t.StagedFiles {
		if f.Path == relPath {
			t.SidebarMode = int(ModeWorkingChanges)
			t.App.Settings.AllChanges = false
			t.SelectFile(f)
			return
		}
	}
}

// TriggerCodeSearchAsync 异步执行项目文件内容全文搜索（带序列号防竞态）。
// 无输入参数，无返回值。
func (t *RepoTab) TriggerCodeSearchAsync() {
	q := strings.TrimSpace(t.ProjectSearchQuery)
	seq := t.searchSeq.Add(1)
	if q == "" {
		t.CodeSearchMatches = nil
		t.CodeSearching = false
		return
	}

	t.CodeSearching = true
	caseSens := t.SearchCaseSensitive
	incIgn := t.SearchIncludeIgnore

	go func() {
		matches, _ := t.Client.SearchProjectCode(q, caseSens, incIgn, 300)
		if t.searchSeq.Load() != seq {
			return
		}
		t.App.RunUI(func() {
			if t.searchSeq.Load() != seq {
				return
			}
			t.CodeSearching = false
			t.CodeSearchMatches = matches
		})
	}()
}

// ReloadProjectFileListAsync 异步重新加载项目所有文件列表（当用户切换“包含忽略文件”或仓库刷新时调用）。
// 无输入参数，无返回值。
func (t *RepoTab) ReloadProjectFileListAsync() {
	incIgn := t.SearchIncludeIgnore
	go func() {
		list, err := t.Client.ListProjectFiles(incIgn)
		if err != nil {
			return
		}
		t.App.RunUI(func() {
			t.ProjectFiles = list
			if t.SelectedProjectPath == "" && len(list) > 0 {
				t.OpenProjectFile(list[0], 0, "")
			}
		})
	}()
}

// ActiveFileList 返回当前模式下（工作区或某次历史 Commit）可浏览的有序变更文件列表。
// 无输入参数，返回当前激活的 ChangedFile 切片。
func (t *RepoTab) ActiveFileList() []git.ChangedFile {
	if SidebarMode(t.SidebarMode) == ModeCommitHistory {
		return FilterFiles(t.CommitFiles, t.SearchQuery)
	}
	var all []git.ChangedFile
	all = append(all, FilterFiles(t.StagedFiles, t.SearchQuery)...)
	all = append(all, FilterFiles(t.UnstagedFiles, t.SearchQuery)...)
	return all
}

// SelectAdjacentFile 切换到当前文件列表中的上一个（delta=-1）或下一个（delta=+1）变更文件。
// 输入参数 delta 为步进偏移量，无返回值。
func (t *RepoTab) SelectAdjacentFile(delta int) {
	files := t.ActiveFileList()
	if len(files) == 0 {
		return
	}
	curIdx := -1
	for i, f := range files {
		if f.Key() == t.SelectedFileKey {
			curIdx = i
			break
		}
	}
	nextIdx := 0
	if curIdx >= 0 {
		nextIdx = (curIdx + delta + len(files)) % len(files)
	}
	t.SelectFile(files[nextIdx])
}

// SelectFile 选中指定的变更文件并在后台确保其 Diff 数据已加载。
// 输入参数 f 为目标变更文件结构体，无返回值。
func (t *RepoTab) SelectFile(f git.ChangedFile) {
	key := f.Key()
	t.SelectedFileKey = key
	if _, ok := t.DiffEntries[key]; !ok {
		t.loadSingleDiffAsync(f, false)
	}
}

// SelectCommit 选中历史记录中的某次 Commit 并异步加载该 Commit 的变更文件列表与 Diff。
// 输入参数 commitHash 为目标提交哈希，无返回值。
func (t *RepoTab) SelectCommit(commitHash string) {
	if t.SelectedCommitHash == commitHash && len(t.CommitFiles) > 0 {
		return
	}
	t.SelectedCommitHash = commitHash
	go func() {
		files, err := t.Client.CommitFiles(commitHash)
		if err != nil {
			t.setBanner(err.Error(), true)
			return
		}
		var bundles []git.FileDiffBundle
		for _, f := range files {
			b, err := t.Client.GetCommitFileDiff(commitHash, f)
			if err == nil {
				bundles = append(bundles, b)
			}
		}
		t.App.RunUI(func() {
			t.CommitFiles = files
			for _, b := range bundles {
				t.applyDiffBundle("commit:"+commitHash+":"+b.File.Path, b, true)
			}
			if len(files) > 0 {
				t.SelectedFileKey = files[0].Key()
			} else {
				t.SelectedFileKey = ""
			}
		})
	}()
}

// CurrentEntryKey 根据当前所处模式（工作区或历史提交）生成 DiffEntries 字典的查找键。
// 输入参数 f 为目标变更文件，返回唯一缓存键字符串。
func (t *RepoTab) CurrentEntryKey(f git.ChangedFile) string {
	if SidebarMode(t.SidebarMode) == ModeCommitHistory {
		return "commit:" + t.SelectedCommitHash + ":" + f.Path
	}
	return f.Key()
}

// RefreshAsync 将状态刷新请求交给 UI 线程。forceOverwriteEdited 为 true 时允许覆盖未保存编辑，无返回值。
func (t *RepoTab) RefreshAsync(forceOverwriteEdited bool) {
	t.App.RunUI(func() {
		t.startRefresh(forceOverwriteEdited)
	})
}

// refreshManually 从 UI 线程刷新仓库，并清除上一次操作的提示。无输入参数，无返回值。
func (t *RepoTab) refreshManually() {
	if !t.Busy {
		t.StatusBanner = ""
		t.IsBannerError = false
	}
	t.startRefresh(false)
}

// startRefresh 从 UI 线程启动刷新，先发布 Git 状态，再读取其他仓库数据。
// forceOverwriteEdited 表示是否允许覆盖未保存编辑，无返回值。
func (t *RepoTab) startRefresh(forceOverwriteEdited bool) {
	if t.closed {
		return
	}
	seq := t.refreshSeq.Add(1)
	t.refreshForce = t.refreshForce || forceOverwriteEdited
	includeIgnored := t.SearchIncludeIgnore
	go func() {
		status, err := t.Client.Status()
		refreshedAt := time.Now()
		if t.refreshSeq.Load() != seq {
			return
		}
		t.App.RunUI(func() {
			if t.refreshSeq.Load() != seq {
				return
			}
			if err != nil {
				t.StatusBanner = err.Error()
				t.IsBannerError = true
				t.refreshForce = false
				return
			}
			t.lastRefreshAt = refreshedAt
			t.applyWorkingStatus(seq, status)
			go t.refreshDetails(seq, status, includeIgnored)
		})
	}()
}

// applyWorkingStatus 根据 seq 对应的 Git 状态更新文件分组、选中项和 Diff 缓存，无返回值。
func (t *RepoTab) applyWorkingStatus(seq uint64, status git.StatusResult) {
	t.workingStatusSeq = seq
	t.StagedFiles = status.Staged
	t.UnstagedFiles = status.Unstaged

	activeKeys := make(map[string]bool, len(status.Staged)+len(status.Unstaged))
	for _, f := range status.Staged {
		activeKeys[f.Key()] = true
	}
	for _, f := range status.Unstaged {
		activeKeys[f.Key()] = true
	}
	for key := range t.DiffEntries {
		if !strings.HasPrefix(key, "commit:") && !activeKeys[key] {
			delete(t.DiffEntries, key)
		}
	}

	if SidebarMode(t.SidebarMode) == ModeWorkingChanges && !activeKeys[t.SelectedFileKey] {
		switch {
		case len(status.Unstaged) > 0:
			t.SelectedFileKey = status.Unstaged[0].Key()
		case len(status.Staged) > 0:
			t.SelectedFileKey = status.Staged[0].Key()
		default:
			t.SelectedFileKey = ""
		}
	}
}

// refreshDetails 在后台读取 seq 对应的分支、历史、项目文件和 Diff。
// status 决定预加载的文件，includeIgnored 控制项目文件扫描范围，无返回值。
func (t *RepoTab) refreshDetails(seq uint64, status git.StatusResult, includeIgnored bool) {
	branchInfo, _ := t.Client.GetBranchInfo()
	if t.refreshSeq.Load() != seq {
		return
	}
	commits, _ := t.Client.RecentCommits(50)
	if t.refreshSeq.Load() != seq {
		return
	}
	projectFiles, projectErr := t.Client.ListProjectFiles(includeIgnored)

	// 保留原来的预加载上限和顺序，未预加载的 Diff 按需读取。
	stagedCount := min(len(status.Staged), 25)
	files := append([]git.ChangedFile(nil), status.Staged[:stagedCount]...)
	files = append(files, status.Unstaged[:min(len(status.Unstaged), 25-stagedCount)]...)
	var bundles []git.FileDiffBundle
	for _, f := range files {
		if t.refreshSeq.Load() != seq {
			return
		}
		if b, err := t.Client.GetWorkingFileDiff(f); err == nil {
			bundles = append(bundles, b)
		}
	}

	t.App.RunUI(func() {
		if t.refreshSeq.Load() != seq {
			return
		}
		t.BranchInfo = branchInfo
		t.SelectedBranch = branchInfo.Current
		t.Commits = commits
		if projectErr == nil && t.SearchIncludeIgnore == includeIgnored {
			t.ProjectFiles = projectFiles
		}
		for _, b := range bundles {
			t.applyDiffBundle(b.File.Key(), b, t.refreshForce)
		}
		t.refreshForce = false

		switch SidebarMode(t.SidebarMode) {
		case ModeCommitHistory:
			if t.SelectedCommitHash == "" && len(t.Commits) > 0 {
				t.SelectCommit(t.Commits[0].Hash)
			}
		case ModeProjectFiles:
			if t.SelectedProjectPath == "" && len(t.ProjectFiles) > 0 {
				t.OpenProjectFile(t.ProjectFiles[0], 0, "")
			}
		}
	})
}

// loadSingleDiffAsync 按需加载文件 f 的 Diff，同一状态快照内合并重复请求。
// forceOverwrite 表示是否覆盖未保存编辑，无返回值。
func (t *RepoTab) loadSingleDiffAsync(f git.ChangedFile, forceOverwrite bool) {
	key := t.CurrentEntryKey(f)
	commit := ""
	load := diffLoad{statusSeq: t.workingStatusSeq, force: forceOverwrite}
	if SidebarMode(t.SidebarMode) == ModeCommitHistory {
		commit = t.SelectedCommitHash
		load.statusSeq = 0
	}
	if current, ok := t.loadingDiffs[key]; ok && current.statusSeq == load.statusSeq && (current.force || !load.force) {
		return
	}
	t.loadingDiffs[key] = load
	go func() {
		var b git.FileDiffBundle
		var err error
		if commit != "" {
			b, err = t.Client.GetCommitFileDiff(commit, f)
		} else {
			b, err = t.Client.GetWorkingFileDiff(f)
		}
		t.App.RunUI(func() {
			if t.loadingDiffs[key] != load {
				return
			}
			delete(t.loadingDiffs, key)
			if err == nil && !t.closed && (commit != "" || t.workingStatusSeq == load.statusSeq) {
				t.applyDiffBundle(key, b, forceOverwrite)
			}
		})
	}()
}

// applyDiffBundle 在 UI 主线程上将查询到的 FileDiffBundle 转换为或更新到对应的 editor.DiffState。
// 输入参数 key 为缓存键，b 为差异数据包，forceOverwrite 为是否强制覆盖未保存编辑；无返回值。
func (t *RepoTab) applyDiffBundle(key string, b git.FileDiffBundle, forceOverwrite bool) {
	existing, ok := t.DiffEntries[key]
	if ok && existing.IsEdited() && !forceOverwrite {
		if existing.BaseNewText != b.NewText || existing.BaseOldText != b.OldText {
			existing.DiskChanged = true
		}
		return
	}

	if ok && !existing.DiskChanged &&
		existing.BaseOldText == b.OldText &&
		existing.BaseNewText == b.NewText &&
		existing.File.Status == b.File.Status &&
		existing.IsBinary == b.IsBinary {
		existing.File = b.File
		existing.syncLayoutFromSettings(t.App.Settings.UnifiedDiff)
		return
	}

	diffState := &editor.DiffState{
		Editable:       b.Editable,
		HunkActions:    !strings.HasPrefix(key, "commit:"),
		ScrollPastEnd:  true,
		KeepScrollbars: false,
	}
	if t.App.Settings.UnifiedDiff {
		diffState.Layout = editor.DiffUnified
	} else {
		diffState.Layout = editor.DiffSplit
	}

	lang := editor.LanguageForText(b.File.Path, b.NewText)
	if lang == nil {
		lang = editor.LanguageFor(b.File.Path)
	}

	appliedViaPatch := false
	if b.Patch != nil && !b.IsBinary {
		if err := diffState.SetPatchFiles(*b.Patch, b.OldText, b.NewText); err == nil {
			appliedViaPatch = true
		}
	}
	if !appliedViaPatch {
		diffState.Old = editor.NewBuffer(b.OldText)
		diffState.New = editor.NewBuffer(b.NewText)
	}
	if lang != nil {
		if diffState.Old != nil {
			diffState.Old.SetLanguage(lang)
		}
		if diffState.New != nil {
			diffState.New.SetLanguage(lang)
		}
	}

	entry := &FileDiffEntry{
		File:        b.File,
		Diff:        diffState,
		BaseOldText: b.OldText,
		BaseNewText: b.NewText,
		IsBinary:    b.IsBinary,
		Editable:    b.Editable,
		DiskChanged: false,
	}
	t.bindHunkActions(entry)
	t.DiffEntries[key] = entry
}

// syncLayoutFromSettings 同步全局的分栏/合并视图配置到单个 DiffState。
// 输入参数 unified 为 true 时切换为合并视图，false 为分栏视图。无返回值。
func (e *FileDiffEntry) syncLayoutFromSettings(unified bool) {
	if e == nil || e.Diff == nil {
		return
	}
	if unified {
		e.Diff.Layout = editor.DiffUnified
	} else {
		e.Diff.Layout = editor.DiffSplit
	}
}

// bindHunkActions 为 DiffState 挂载右上角纯中文的代码块级 Lobe 小尺寸快捷操作按钮（暂存此块 / 取消暂存此块 / 放弃此块修改）。
// 输入参数 entry 为当前文件的 FileDiffEntry 指针，无返回值。
func (t *RepoTab) bindHunkActions(entry *FileDiffEntry) {
	if entry == nil || entry.Diff == nil {
		return
	}
	entry.Diff.RenderHunkActions = func(c *ui.Context, bar ui.Element, a *editor.DiffHunkActions) {
		hunk := a.Hunk
		bar.Gap(4).Padding(2, 6).Children(func() {
			if entry.File.Staged {
				if action.Button(c, "取消暂存此块", action.ButtonProps{
					Variant: action.Default,
					Size:    theme.Small,
				}).Clicked() {
					t.runGitHunkAction(entry, hunk, true, true, "已取消暂存该代码块")
				}
			} else {
				if action.Button(c, "放弃此块", action.ButtonProps{
					Variant: action.Default,
					Size:    theme.Small,
				}).Clicked() {
					if entry.File.Untracked {
						a.Revert()
					} else {
						t.runGitHunkAction(entry, hunk, false, true, "已放弃并还原该代码块修改")
					}
				}
				if !entry.File.Untracked {
					if action.Button(c, "暂存此块", action.ButtonProps{
						Variant: action.Primary,
						Size:    theme.Small,
					}).Clicked() {
						t.runGitHunkAction(entry, hunk, true, false, "已暂存该代码块")
					}
				}
			}
		})
	}
}

// runGitHunkAction 在后台执行针对单个代码块的 git apply 操作，并在成功后刷新仓库状态。
// 输入参数 entry 为文件 Diff 项，hunk 为目标差异块，cached 表示是否作用于暂存区，reverse 表示是否反向撤销，msg 为成功提示。
// 无返回值。
func (t *RepoTab) runGitHunkAction(entry *FileDiffEntry, hunk editor.DiffHunk, cached bool, reverse bool, msg string) {
	curNew := entry.BaseNewText
	if entry.Diff != nil && entry.Diff.New != nil {
		curNew = entry.Diff.New.String()
	}
	curOld := entry.BaseOldText
	filePath := entry.File.Path
	isEdited := entry.IsEdited()

	go func() {
		if isEdited && entry.Editable {
			if err := t.Client.SaveWorkingFile(filePath, curNew); err != nil {
				t.setBanner("保存微调失败: "+err.Error(), true)
				return
			}
		}
		if err := t.Client.ApplySingleHunk(filePath, curOld, curNew, hunk, cached, reverse); err != nil {
			t.setBanner(err.Error(), true)
			return
		}
		t.setBanner(msg, false)
		t.RefreshAsync(true)
	}()
}

// SaveEditedFile 保存当前用户在 DiffView 中微调后的文件内容到工作区磁盘，并立即刷新 Diff。
// 输入参数 entry 为被编辑的 FileDiffEntry；无返回值。
func (t *RepoTab) SaveEditedFile(entry *FileDiffEntry) {
	if entry == nil || !entry.Editable || entry.Diff == nil || entry.Diff.New == nil {
		return
	}
	newContent := entry.Diff.New.String()
	relPath := entry.File.Path
	go func() {
		if err := t.Client.SaveWorkingFile(relPath, newContent); err != nil {
			t.setBanner("保存文件失败: "+err.Error(), true)
			return
		}
		t.setBanner(fmt.Sprintf("已保存 %s", relPath), false)
		t.RefreshAsync(true)
	}()
}

// setBanner 在主线程更新顶部的状态提示条。
// 输入参数 msg 为提示文本，isErr 标记是否为错误消息；无返回值。
func (t *RepoTab) setBanner(msg string, isErr bool) {
	t.App.RunUI(func() {
		t.StatusBanner = msg
		t.IsBannerError = isErr
	})
}

// RunAsyncTask 在后台执行耗时的 Git 命令（如 Commit / Pull / Push / Fetch），期间设置 Busy 状态并在完成后刷新界面。
// 输入参数 label 为执行期间状态栏显示的提示，task 为后台任务函数；无返回值。
func (t *RepoTab) RunAsyncTask(label string, task func() (string, error)) {
	t.Busy = true
	t.StatusBanner = label + "..."
	t.IsBannerError = false

	go func() {
		msg, err := task()
		t.App.RunUI(func() {
			if t.closed {
				return
			}
			t.Busy = false
			if err != nil {
				t.StatusBanner = err.Error()
				t.IsBannerError = true
			} else {
				t.StatusBanner = msg
				t.IsBannerError = false
			}
			t.startRefresh(true)
		})
	}()
}
