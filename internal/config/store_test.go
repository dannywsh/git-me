package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestLoadRemovesStaleUITestRepos 验证旧测试路径会被清理，普通离线仓库仍保留。t 为当前测试，无返回值。
func TestLoadRemovesStaleUITestRepos(t *testing.T) {
	store := NewStore(t.TempDir())
	tempDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(tempDir, "TestAppNativeUIView9876543210123", "001")
	offline := filepath.Join(t.TempDir(), "another-repo", "001")
	cfg := DefaultSettings()
	cfg.RecentRepos = []string{stale, offline}
	cfg.OpenTabs = []string{stale, offline}
	cfg.ActiveTab = stale
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.RecentRepos, []string{offline}) ||
		!slices.Equal(got.OpenTabs, []string{offline}) || got.ActiveTab != "" {
		t.Fatalf("unexpected retained paths: %+v", got)
	}
}

// TestSaveAndLoadGitPath 验证自定义 Git 可执行文件路径的保存、读取去空白以及空值重置持久化。
func TestSaveAndLoadGitPath(t *testing.T) {
	store := NewStore(t.TempDir())
	cfg := DefaultSettings()
	cfg.GitPath = "  /custom/bin/git  "
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.GitPath != "/custom/bin/git" {
		t.Fatalf("got GitPath = %q, want %q", got.GitPath, "/custom/bin/git")
	}

	got.GitPath = ""
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.GitPath != "" {
		t.Fatalf("expected empty GitPath after reset, got %q", reloaded.GitPath)
	}
}
