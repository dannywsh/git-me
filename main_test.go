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
	repoDir := initTempGitRepo(t)
	application := app.NewApp([]string{repoDir})
	defer func() {
		for _, tab := range application.Tabs {
			tab.Close()
		}
	}()

	tt := ui.NewTester(application.View, 1280, 800)
	if !tt.HasText("+ 打开目录") {
		t.Errorf("expected top bar '+ 打开目录' button, texts: %v", tt.Texts())
	}
	if !tt.HasText("代码变更") || !tt.HasText("提交历史") || !tt.HasText("项目文件") {
		t.Errorf("expected sidebar mode segmented controls, texts: %v", tt.Texts())
	}

	// 测试列出项目文件与全文代码搜索
	client, err := git.NewClient(repoDir)
	if err != nil {
		t.Fatal(err)
	}
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
