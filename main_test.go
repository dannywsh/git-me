package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"git-me/internal/app"
	"git-me/internal/config"
	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// initTempGitRepo 在临时目录中初始化一个包含一次初始提交和一处工作区修改的测试 Git 仓库。
// 输入参数 t 为 *testing.T；返回临时仓库的根目录路径。
func initTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Tester")

	file1 := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(file1, []byte("package main\n\nfunc main() {\n\tprintln(\"v1\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "hello.go")
	runGit("commit", "-m", "initial commit")

	// 制造一处工作区修改和一个未跟踪新文件
	if err := os.WriteFile(file1, []byte("package main\n\nfunc main() {\n\tprintln(\"v2 modified by agent\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "internal", "service")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "api.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

// TestGitClientAndHunkOperations 测试 Git 客户端的状态解析、Diff 获取以及单 Hunk 暂存与回滚功能。
func TestGitClientAndHunkOperations(t *testing.T) {
	repoDir := initTempGitRepo(t)
	client, err := git.NewClient(repoDir)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	st, err := client.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(st.Unstaged) != 2 {
		t.Fatalf("expected 2 unstaged files, got %d", len(st.Unstaged))
	}

	// 测试单 Hunk Stage
	var helloFile git.ChangedFile
	for _, f := range st.Unstaged {
		if f.Path == "hello.go" {
			helloFile = f
			break
		}
	}
	bundle, err := client.GetWorkingFileDiff(helloFile)
	if err != nil {
		t.Fatalf("GetWorkingFileDiff failed: %v", err)
	}
	if bundle.Patch == nil {
		t.Fatalf("expected non-nil patch for hello.go")
	}

	ds := &editor.DiffState{}
	if err := ds.SetPatchFiles(*bundle.Patch, bundle.OldText, bundle.NewText); err != nil {
		t.Fatalf("SetPatchFiles failed: %v", err)
	}
	hunks := ds.Hunks()
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	if err := client.ApplySingleHunk("hello.go", bundle.OldText, bundle.NewText, hunks[0], true, false); err != nil {
		t.Fatalf("ApplySingleHunk stage failed: %v", err)
	}

	stAfter, err := client.Status()
	if err != nil {
		t.Fatalf("Status after hunk stage failed: %v", err)
	}
	if len(stAfter.Staged) != 1 || stAfter.Staged[0].Path != "hello.go" {
		t.Fatalf("expected hello.go to be staged after hunk stage, got %+v", stAfter.Staged)
	}
}

// TestAppNativeUIView 使用 MyGo 的无窗口 Tester 验证 Git-Me 主界面渲染与视图切换交互。
func TestAppNativeUIView(t *testing.T) {
	configDir := t.TempDir()
	globalConfig := filepath.Join(configDir, "global.gitconfig")
	systemConfig := filepath.Join(configDir, "system.gitconfig")
	if err := os.WriteFile(globalConfig, []byte("[alias]\n  review = log --oneline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemConfig, []byte("[init]\n  defaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", systemConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	repoDir := initTempGitRepo(t)
	client, err := git.NewClient(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	const remoteURL = "https://example.com/team/project.git"
	for _, args := range [][]string{
		{"remote", "add", "origin", "https://demo:fake-test-token@example.com/team/project.git?token=fake-test-token"},
		{"config", "remote.origin.promisor", "true"},
		{"config", "remote.origin.partialclonefilter", "blob:none"},
		{"config", "http.extraHeader", "Authorization: Bearer fake-test-token"},
		{"config", "sendemail.sample.smtpPass", "fixture-smtp-password"},
		{"config", "imap.pass", "fixture-imap-password"},
		{"config", "extensions.worktreeConfig", "true"},
		{"config", "--worktree", "merge.conflictStyle", "diff3"},
	} {
		if _, err := client.Run(args...); err != nil {
			t.Fatal(err)
		}
	}
	application := &app.App{Settings: config.DefaultSettings(), Store: config.NewStore(t.TempDir())}
	if err := application.OpenRepo(repoDir, true); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, tab := range application.Tabs {
			tab.Close()
		}
	}()

	tt := ui.NewTester(application.View, 1280, 800)
	open, hasOpen := tt.AccessByRole(ui.RoleButton, "打开目录")
	closeTab, hasClose := tt.AccessByRole(ui.RoleButton, "关闭此仓库标签页")
	info, hasInfo := tt.AccessByRole(ui.RoleButton, "仓库详情")
	if !hasOpen || !hasClose || !hasInfo || open.Bounds.X-closeTab.Bounds.X > 40 ||
		open.Bounds.X <= closeTab.Bounds.X || info.Bounds.X < 1200 {
		t.Fatalf("unexpected tab bar placement: add=%v close=%v info=%v", open.Bounds, closeTab.Bounds, info.Bounds)
	}
	if !tt.HasText("代码变更") || !tt.HasText("提交历史") || !tt.HasText("项目文件") {
		t.Errorf("expected sidebar mode segmented controls, texts: %v", tt.Texts())
	}
	waitForText := func(label string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !tt.HasText(label) && time.Now().Before(deadline) {
			tt.Frame()
			time.Sleep(10 * time.Millisecond)
		}
		if !tt.HasText(label) {
			t.Fatalf("missing %q: %v", label, tt.Texts())
		}
	}
	waitForText(remoteURL)
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	waitForText("本项目")
	waitForText("远程与同步")
	waitForText("remote.origin.url")
	waitForText("差异与合并")
	waitForText("merge.conflictstyle")
	waitForText("网络与认证")
	waitForText("http.extraheader")
	waitForText("••••••")
	waitForText("sendemail.sample.smtppass")
	waitForText("imap.pass")
	for _, credential := range []string{"fake-test-token", "fixture-smtp-password", "fixture-imap-password"} {
		if tt.HasText(credential) {
			t.Fatal("repository info exposed a credential fixture")
		}
	}
	if local, ok := tt.AccessByRole(ui.RoleTab, "本项目"); !ok || !local.Selected ||
		tt.HasText("alias.review") || tt.HasText("init.defaultbranch") {
		t.Fatalf("repository config should be the default and show only local/worktree entries: %v", tt.Texts())
	}
	for _, check := range []struct {
		tab, visible, hidden string
	}{
		{"全局", "alias.review", "remote.origin.url"},
		{"其他来源", "init.defaultbranch", "alias.review"},
		{"仓库概览", "Git 目录", "init.defaultbranch"},
		{"全局", "命令与显示", "Git 目录"},
	} {
		if err := tt.Click(check.tab); err != nil {
			t.Fatal(err)
		}
		waitForText(check.visible)
		if tt.HasText(check.hidden) {
			t.Fatalf("unexpected %q in %s: %v", check.hidden, check.tab, tt.Texts())
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	waitForText("remote.origin.url")
	tt.Key(0, ui.KeyEscape)
	tt.Advance(500 * time.Millisecond)
	if err := tt.Click("最近仓库"); err != nil {
		t.Fatal(err)
	}
	remove, ok := tt.AccessByRole(ui.RoleButton, "从最近仓库移除 "+filepath.Base(repoDir))
	if !ok {
		t.Fatalf("missing recent remove action: %v", tt.Texts())
	}
	x, y := remove.Bounds.X+remove.Bounds.W/2, remove.Bounds.Y+remove.Bounds.H/2
	beforeHover := tt.Image().RGBAAt(int(x), int(y))
	tt.Move(x, y)
	tt.Advance(200 * time.Millisecond)
	if afterHover := tt.Image().RGBAAt(int(x), int(y)); afterHover == beforeHover {
		t.Error("recent remove action did not change appearance on hover")
	}
	tt.ClickAt(x, y)
	waitForText("暂无最近仓库")
	saved, err := application.Store.Load()
	if err != nil || len(saved.RecentRepos) != 0 || len(saved.OpenTabs) != 1 || application.ActiveTab() == nil {
		t.Fatalf("removing history must keep the active tab open: %+v, %v", saved, err)
	}

	// 测试列出项目文件与全文代码搜索
	files, err := client.ListProjectFiles(false)
	if err != nil || len(files) < 2 {
		t.Fatalf("expected at least 2 project files, got %v (err=%v)", files, err)
	}
	matches, err := client.SearchProjectCode("modified by agent", false, false, 50)
	if err != nil || len(matches) != 1 || matches[0].Path != "hello.go" {
		t.Fatalf("expected code match in hello.go, got %+v (err=%v)", matches, err)
	}
}

// TestWorkingStatusUpdatesBeforeDetails 验证详情读取被阻塞时，UI 仍能完成暂存分组更新和提交。
// 输入参数 t 为当前测试，无返回值。
func TestWorkingStatusUpdatesBeforeDetails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git command gate uses a POSIX shell wrapper")
	}
	repoDir := initTempGitRepo(t)
	client, err := git.NewClient(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	started := filepath.Join(bin, "started")
	release := filepath.Join(bin, "release")
	// 暂停项目文件扫描，直到测试观察到两次状态变化。
	wrapper := `#!/bin/sh
if [ "$4" = ls-files ]; then
    : > "$GIT_ME_TEST_STARTED"
    for i in 1 2 3 4 5 6 7 8 9 10; do
        [ ! -f "$GIT_ME_TEST_RELEASE" ] || break
        sleep 1
    done
fi
exec "$GIT_ME_TEST_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_ME_TEST_GIT", gitPath)
	t.Setenv("GIT_ME_TEST_STARTED", started)
	t.Setenv("GIT_ME_TEST_RELEASE", release)
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o644) })

	application := &app.App{Settings: config.DefaultSettings(), Store: config.NewStore(t.TempDir())}
	tab := app.NewRepoTab(application, client)
	defer tab.Close()
	tab.Watcher.Stop()
	application.Tabs = []*app.RepoTab{tab}
	tt := ui.NewTester(application.View, 1280, 800)

	waitFor := func(description string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			if ready() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("waiting for %s: %v", description, tt.Texts())
	}
	waitFor("initial details to start", func() bool {
		_, err := os.Stat(started)
		return err == nil
	})
	if len(tab.UnstagedFiles) != 2 {
		t.Fatalf("expected working changes before details, got %v", tab.UnstagedFiles)
	}

	tab.RunAsyncTask("正在暂存全部更改", func() (string, error) {
		return "已暂存全部更改", client.StageAll()
	})
	waitFor("both files to move into the staged group", func() bool {
		return tt.HasText("提交 (2)") && len(tab.StagedFiles) == 2 && len(tab.UnstagedFiles) == 0
	})
	tab.CommitMessage = "提交暂存文件"
	tt.Frame()
	if err := tt.Click("提交 (2)"); err != nil {
		t.Fatal(err)
	}
	waitFor("commit to clear the staged group", func() bool {
		return tt.HasText("提交 (0)") && !tab.Busy && len(tab.StagedFiles) == 0 && len(tab.UnstagedFiles) == 0
	})
	if tab.StatusBanner != "" || !tt.HasText("刚刚") {
		t.Fatalf("expected refresh time without a commit banner, got %q: %v", tab.StatusBanner, tt.Texts())
	}
	if len(tab.Commits) != 0 {
		t.Fatalf("details finished before the gate was released: %v", tab.Commits)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("latest commit details", func() bool {
		return len(tab.Commits) == 2 && tab.Commits[0].Subject == "提交暂存文件"
	})
}
