package app

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git-me/internal/config"
	"git-me/internal/git"

	"github.com/egoist/mygo/ui"
)

// TestOpenRepoErrorVisibility 验证静默探测非 Git 目录时不污染欢迎页错误提示，显式打开或缺少 Git 时正常展示具体原因。
func TestOpenRepoErrorVisibility(t *testing.T) {
	nonGitDir := t.TempDir()
	a := &App{
		Settings: config.DefaultSettings(),
		Store:    config.NewStore(t.TempDir()),
	}

	if err := a.OpenRepo(nonGitDir, false); err == nil {
		t.Fatal("expected OpenRepo on non-git dir to return error")
	}
	if a.GlobalError != "" {
		t.Fatalf("silent OpenRepo should not set GlobalError, got %q", a.GlobalError)
	}

	if err := a.OpenRepo(nonGitDir, true); err == nil {
		t.Fatal("expected active OpenRepo on non-git dir to return error")
	}
	if a.GlobalError == "" || strings.Contains(a.GlobalError, "()") {
		t.Fatalf("unexpected GlobalError after active OpenRepo: %q", a.GlobalError)
	}

	t.Setenv("PATH", t.TempDir())
	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramW6432", "")
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("LocalAppData", "")

	a.GlobalError = ""
	err := a.OpenRepo(nonGitDir, false)
	if !errors.Is(err, git.ErrGitNotFound) {
		t.Fatalf("expected ErrGitNotFound when git missing, got %v", err)
	}
	if a.GlobalError != git.ErrGitNotFound.Error() {
		t.Fatalf("GlobalError = %q, want %q", a.GlobalError, git.ErrGitNotFound.Error())
	}
}

// TestNativeSettingsWindowView 验证独立原生设置窗口的 Sidebar 分类切换、Git 路径校验持久化，并确认主界面左下角与欢迎页无设置按钮。
func TestNativeSettingsWindowView(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not in PATH")
	}
	t.Cleanup(func() { git.SetGitPath("") })

	application := &App{
		Settings: config.DefaultSettings(),
		Store:    config.NewStore(t.TempDir()),
	}
	defer func() {
		for _, tab := range application.Tabs {
			tab.Close()
		}
	}()

	// 1. 主界面欢迎页和侧边栏均不包含页面内设置入口
	mainTester := ui.NewTester(application.View, 1280, 800)
	if mainTester.HasText("设置 Git 路径") {
		t.Fatalf("welcome page should not render web-style settings button: %v", mainTester.Texts())
	}
	repoDir := t.TempDir()
	initCmd := exec.Command(realGit, "init")
	initCmd.Dir = repoDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}
	if err := application.OpenRepo(repoDir, true); err != nil {
		t.Fatal(err)
	}
	mainTester.Frame()
	if _, hasSidebarSettings := mainTester.AccessByRole(ui.RoleButton, "设置"); hasSidebarSettings {
		t.Fatalf("main sidebar should not contain bottom-left settings button: %v", mainTester.Texts())
	}

	// 2. 使用纯 MyGo 原生组件渲染设置窗口，默认展示左侧拉伸 Sidebar 与 Git 配置表单
	tt := ui.NewTester(application.viewSettingsWindow, 740, 480)
	waitFor := func(desc string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			tt.Frame()
			if ready() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for %s: texts=%v", desc, tt.Texts())
	}

	if !tt.HasText("Git 命令行程序") || !tt.HasText("可执行文件路径") || !tt.HasText("Git") || !tt.HasText("通用") {
		t.Fatalf("missing native settings sidebar or Git card: %v", tt.Texts())
	}
	// 验证左侧 Sidebar 贴顶铺满高度，不再居中悬空
	sidebarNode, hasSidebar := tt.AccessByRole(ui.RoleTree, "程序设置")
	if !hasSidebar || sidebarNode.Bounds.Y > 80 || sidebarNode.Bounds.H < 360 {
		t.Fatalf("expected full-height stretched sidebar near top, got ok=%v bounds=%+v", hasSidebar, sidebarNode.Bounds)
	}

	// 3. 输入无效路径点击保存时提示错误且不修改持久化配置
	application.settingsState.gitPathInput = filepath.Join(t.TempDir(), "missing-git")
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	waitFor("validation error message", func() bool {
		return !application.settingsState.saving && application.settingsState.errorMsg != ""
	})
	if application.Settings.GitPath != "" {
		t.Fatalf("expected empty Settings.GitPath on invalid input, got %q", application.Settings.GitPath)
	}

	// 4. 输入合法 Git 路径保存并验证持久化
	application.settingsState.gitPathInput = realGit
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	waitFor("settings saved", func() bool {
		return !application.settingsState.saving && application.settingsState.statusMsg == "已保存 Git 路径设置"
	})

	wantPath := filepath.Clean(realGit)
	if application.Settings.GitPath != wantPath || git.ConfiguredGitPath() != wantPath {
		t.Fatalf("git path not applied: settings=%q configured=%q want=%q", application.Settings.GitPath, git.ConfiguredGitPath(), wantPath)
	}
	saved, err := application.Store.Load()
	if err != nil || saved.GitPath != wantPath {
		t.Fatalf("saved Settings.GitPath = %q (err=%v), want %q", saved.GitPath, err, wantPath)
	}

	// 5. 切换 Sidebar 到「通用」分类并通过原生 Switch 修改偏好设置
	if err := tt.Click("通用"); err != nil {
		t.Fatal(err)
	}
	waitFor("general section", func() bool {
		return tt.HasText("界面偏好") && tt.HasText("目录树模式")
	})
	sw, hasSwitch := tt.AccessByRole(ui.RoleSwitch, "目录树模式")
	if !hasSwitch {
		t.Fatalf("missing native switch for 目录树模式: %v", tt.Texts())
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	tt.Frame()
	if !application.Settings.TreeMode {
		t.Fatal("expected TreeMode to be enabled after toggling switch")
	}
	saved, err = application.Store.Load()
	if err != nil || !saved.TreeMode {
		t.Fatalf("expected TreeMode persisted, got %+v (err=%v)", saved, err)
	}
}
