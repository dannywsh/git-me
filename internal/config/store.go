package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Settings 保存应用的持久化状态，包括最近打开的仓库列表、当前打开的仓库标签页和界面偏好。
type Settings struct {
	RecentRepos          []string `json:"recent_repos"`
	OpenTabs             []string `json:"open_tabs"`
	ActiveTab            string   `json:"active_tab"`
	SidebarWidth         float32  `json:"sidebar_width"`
	TreeMode             bool     `json:"tree_mode"`
	UnifiedDiff          bool     `json:"unified_diff"`
	AllChanges           bool     `json:"all_changes"`
	CheckUpdatesOnLaunch bool     `json:"check_updates_on_launch"`
	GitPath              string   `json:"git_path,omitempty"`
}

// Store 负责读写本地磁盘上的 JSON 配置文件。
type Store struct {
	path string
}

// NewStore 创建一个配置存储器实例。
// 输入参数 configDir 为配置文件所在的目录路径（为空时使用用户配置目录）。
// 返回初始化后的 *Store 实例。
func NewStore(configDir string) *Store {
	if configDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			dir = os.TempDir()
		}
		configDir = filepath.Join(dir, "git-me")
	}
	return &Store{
		path: filepath.Join(configDir, "settings.json"),
	}
}

// DefaultSettings 返回应用的默认配置项。
// 无输入参数，返回默认初始化的 Settings 结构体。
func DefaultSettings() Settings {
	return Settings{
		RecentRepos:          []string{},
		OpenTabs:             []string{},
		SidebarWidth:         310,
		TreeMode:             false,
		UnifiedDiff:          false,
		AllChanges:           false,
		CheckUpdatesOnLaunch: true,
	}
}

// Load 从磁盘读取持久化配置；如果文件不存在或解析失败，则返回默认配置。
// 无输入参数，返回加载后的 Settings 和可能发生的错误。
func (s *Store) Load() (Settings, error) {
	cfg := DefaultSettings()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultSettings(), err
	}
	cfg.GitPath = strings.TrimSpace(cfg.GitPath)
	if cfg.SidebarWidth < 200 {
		cfg.SidebarWidth = 310
	}
	cfg.RecentRepos = slices.DeleteFunc(cfg.RecentRepos, isStaleUITestRepo)
	cfg.OpenTabs = slices.DeleteFunc(cfg.OpenTabs, isStaleUITestRepo)
	if isStaleUITestRepo(cfg.ActiveTab) {
		cfg.ActiveTab = ""
	}
	return cfg, nil
}

var uiTestRepoPath = regexp.MustCompile(`^TestAppNativeUIView[0-9]+/[0-9]{3}$`)

// isStaleUITestRepo 判断 path 是否是旧界面测试留下的已失效临时仓库路径。
func isStaleUITestRepo(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	tempDir := os.TempDir()
	resolved, _ := filepath.EvalSymlinks(tempDir)
	for _, root := range []string{tempDir, resolved} {
		if root == "" {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err == nil && uiTestRepoPath.MatchString(filepath.ToSlash(relative)) {
			_, err = os.Stat(path)
			return os.IsNotExist(err)
		}
	}
	return false
}

// Save 将最新的配置项原子地写入磁盘 JSON 文件。
// 输入参数 cfg 为待保存的 Settings 结构体，返回写入过程中可能出现的错误。
func (s *Store) Save(cfg Settings) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// AddRecentRepo 将一个仓库路径添加到最近打开列表的最前面，去重并限制最大保留数量为 15 个。
// 输入参数 cfg 为配置指针，repoPath 为规范化后的仓库根路径。无返回值。
func AddRecentRepo(cfg *Settings, repoPath string) {
	cleaned := filepath.Clean(repoPath)
	RemoveRecentRepo(cfg, cleaned)
	cfg.RecentRepos = append([]string{cleaned}, cfg.RecentRepos...)
	if len(cfg.RecentRepos) > 15 {
		cfg.RecentRepos = cfg.RecentRepos[:15]
	}
}

// RemoveRecentRepo 从 cfg 删除与 repoPath 对应的历史项，无返回值。
func RemoveRecentRepo(cfg *Settings, repoPath string) {
	cleaned := filepath.Clean(repoPath)
	cfg.RecentRepos = slices.DeleteFunc(cfg.RecentRepos, func(p string) bool {
		return filepath.Clean(p) == cleaned
	})
}
