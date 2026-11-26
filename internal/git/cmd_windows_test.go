//go:build windows

package git

import (
	"os/exec"
	"slices"
	"testing"
)

// TestConfigureCommandHidesWindow 验证 Windows 平台下为 git 子进程同时设置 HideWindow 与 CREATE_NO_WINDOW。
func TestConfigureCommandHidesWindow(t *testing.T) {
	cmd := exec.Command("git", "--version")
	configureCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected non-nil SysProcAttr on Windows")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected HideWindow to be true on Windows")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("expected CreationFlags to include CREATE_NO_WINDOW (0x%x), got 0x%x", createNoWindow, cmd.SysProcAttr.CreationFlags)
	}
}

// TestFallbackGitPaths 验证 Windows 候选 Git 路径生成与去重逻辑。
func TestFallbackGitPaths(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	t.Setenv("LocalAppData", `C:\Users\tester\AppData\Local`)

	paths := fallbackGitPaths()
	want := []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Users\tester\AppData\Local\Programs\Git\cmd\git.exe`,
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("fallbackGitPaths = %v, want %v", paths, want)
	}
}
