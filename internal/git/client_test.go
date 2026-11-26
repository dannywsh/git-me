package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestNewGitCommandEnvironment 验证 newGitCommand 正确设置工作目录及无交互环境变量。
func TestNewGitCommandEnvironment(t *testing.T) {
	dir := t.TempDir()
	cmd, err := newGitCommand(t.Context(), dir, "--version")
	if err != nil {
		t.Fatalf("newGitCommand failed: %v", err)
	}
	if cmd.Dir != dir {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, dir)
	}
	if !slices.Contains(cmd.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("missing GIT_TERMINAL_PROMPT=0 in env")
	}
	if !slices.Contains(cmd.Env, "LC_ALL=C") {
		t.Fatalf("missing LC_ALL=C in env")
	}
}

// TestResolveGitExecutableNotFound 验证缺少 git 命令时 NewClient 返回明确的 ErrGitNotFound，且可通过兜底路径找到 git。
func TestResolveGitExecutableNotFound(t *testing.T) {
	origFallback := gitFallbackPaths
	t.Cleanup(func() { gitFallbackPaths = origFallback })

	t.Setenv("PATH", t.TempDir())
	gitFallbackPaths = func() []string { return nil }

	_, err := NewClient(t.TempDir())
	if !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("NewClient error = %v, want ErrGitNotFound", err)
	}
	if strings.Contains(err.Error(), "()") {
		t.Fatalf("error message should not contain empty parens: %q", err.Error())
	}

	fallbackBin := filepath.Join(t.TempDir(), "git-custom")
	if err := os.WriteFile(fallbackBin, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitFallbackPaths = func() []string { return []string{fallbackBin} }

	resolved, err := resolveGitExecutable()
	if err != nil {
		t.Fatalf("resolveGitExecutable fallback failed: %v", err)
	}
	if resolved != fallbackBin {
		t.Fatalf("resolved = %q, want %q", resolved, fallbackBin)
	}
}

// TestFormatTopLevelError 验证各种失败场景下格式化仓库校验错误时不会产生空括号。
func TestFormatTopLevelError(t *testing.T) {
	cases := []struct {
		name     string
		out      []byte
		err      error
		ctxErr   error
		wantIs   error
		wantText string
	}{
		{
			name:   "git not found sentinel",
			err:    ErrGitNotFound,
			wantIs: ErrGitNotFound,
		},
		{
			name:   "exec not found",
			err:    exec.ErrNotFound,
			wantIs: ErrGitNotFound,
		},
		{
			name:     "context timeout",
			err:      context.DeadlineExceeded,
			ctxErr:   context.DeadlineExceeded,
			wantText: "检测 Git 仓库超时",
		},
		{
			name:     "command output present",
			out:      []byte("fatal: not a git repository (or any of the parent directories): .git\n"),
			err:      errors.New("exit status 128"),
			wantText: "当前目录不是有效的 Git 仓库 (fatal: not a git repository",
		},
		{
			name:     "empty output falls back to err",
			out:      nil,
			err:      errors.New("exit status 1"),
			wantText: "当前目录不是有效的 Git 仓库 (exit status 1)",
		},
		{
			name:     "whitespace only falls back cleanly",
			out:      []byte("  \n\t "),
			err:      errors.New(" "),
			wantText: "当前目录不是有效的 Git 仓库",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTopLevelError(tc.out, tc.err, tc.ctxErr)
			if tc.wantIs != nil && !errors.Is(got, tc.wantIs) {
				t.Fatalf("formatTopLevelError = %v, want %v", got, tc.wantIs)
			}
			if tc.wantText != "" && !strings.Contains(got.Error(), tc.wantText) {
				t.Fatalf("formatTopLevelError = %q, want substring %q", got.Error(), tc.wantText)
			}
			if strings.Contains(got.Error(), "()") {
				t.Fatalf("formatTopLevelError produced empty parens: %q", got.Error())
			}
		})
	}
}

// TestRemotesWithCRLF 验证包含 Windows CRLF 行尾的 git remote -v 输出能够被正确解析并脱敏。
func TestRemotesWithCRLF(t *testing.T) {
	raw := "origin\thttps://user:secret@example.com/team/repo.git (fetch)\r\n" +
		"origin\thttps://user:secret@example.com/team/repo.git (push)\r\n"
	remotes := parseRemotesOutput(raw)
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %+v", remotes)
	}
	if remotes[0].Direction != "fetch" || remotes[0].URL != "https://example.com/team/repo.git" {
		t.Fatalf("unexpected fetch remote: %+v", remotes[0])
	}
	if remotes[1].Direction != "push" || remotes[1].URL != "https://example.com/team/repo.git" {
		t.Fatalf("unexpected push remote: %+v", remotes[1])
	}
}

// TestSetGitPathAndResolve 验证自定义 Git 路径优先于系统 PATH、无效路径报错以及置空后恢复自动检测。
func TestSetGitPathAndResolve(t *testing.T) {
	t.Cleanup(func() { SetGitPath("") })

	customBin := filepath.Join(t.TempDir(), "custom-git")
	if err := os.WriteFile(customBin, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	SetGitPath(" " + customBin + " ")
	if ConfiguredGitPath() != customBin {
		t.Fatalf("ConfiguredGitPath = %q, want %q", ConfiguredGitPath(), customBin)
	}

	resolved, err := resolveGitExecutable()
	if err != nil || resolved != customBin {
		t.Fatalf("resolveGitExecutable = (%q, %v), want (%q, nil)", resolved, err, customBin)
	}

	missingPath := filepath.Join(t.TempDir(), "missing-git")
	SetGitPath(missingPath)
	if _, err := resolveGitExecutable(); !errors.Is(err, ErrGitNotFound) {
		t.Fatalf("expected ErrGitNotFound for missing custom path, got %v", err)
	}

	SetGitPath("")
	if ConfiguredGitPath() != "" {
		t.Fatalf("expected empty ConfiguredGitPath after reset, got %q", ConfiguredGitPath())
	}
	autoBin, err := resolveGitExecutable()
	if err != nil || autoBin == "" {
		t.Fatalf("expected auto git resolution after reset, got (%q, %v)", autoBin, err)
	}
}

// TestValidateGitExecutable 验证 Git 可执行文件校验：覆盖自动检测、有效路径、目录、缺失文件、包装器误选及非 Git 程序。
func TestValidateGitExecutable(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not in PATH")
	}

	autoPath, autoVer, err := ValidateGitExecutable("")
	if err != nil || autoPath == "" || !strings.Contains(strings.ToLower(autoVer), "git version") {
		t.Fatalf("ValidateGitExecutable(\"\") = (%q, %q, %v)", autoPath, autoVer, err)
	}

	resolved, ver, err := ValidateGitExecutable(realGit)
	if err != nil || resolved != filepath.Clean(realGit) || !strings.Contains(strings.ToLower(ver), "git version") {
		t.Fatalf("ValidateGitExecutable(realGit) = (%q, %q, %v)", resolved, ver, err)
	}

	if _, _, err := ValidateGitExecutable(t.TempDir()); err == nil {
		t.Fatal("expected error when directory is passed")
	}

	if _, _, err := ValidateGitExecutable(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error when file does not exist")
	}

	wrapper := filepath.Join(t.TempDir(), "git-bash.exe")
	if err := os.WriteFile(wrapper, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateGitExecutable(wrapper); err == nil || !strings.Contains(err.Error(), "命令行程序") {
		t.Fatalf("expected wrapper rejection error, got %v", err)
	}

	notGit := filepath.Join(t.TempDir(), "not-git")
	if err := os.WriteFile(notGit, []byte("#!/bin/sh\necho not-git\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateGitExecutable(notGit); err == nil {
		t.Fatal("expected error for non-git executable")
	}
}
