package git

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FileStatusCode 表示单个文件在 Git 中的变更类型。
type FileStatusCode string

const (
	StatusModified  FileStatusCode = "M"
	StatusAdded     FileStatusCode = "A"
	StatusDeleted   FileStatusCode = "D"
	StatusRenamed   FileStatusCode = "R"
	StatusUntracked FileStatusCode = "U"
	StatusConflict  FileStatusCode = "C"
)

// ChangedFile 描述一个在暂存区（Staged）或工作区（Unstaged）中发生变化的文件条目。
type ChangedFile struct {
	Path      string         // 当前相对于仓库根目录的路径（正斜杠分隔）
	OrigPath  string         // 重命名或复制前的原始相对路径（非重命名时为空）
	Status    FileStatusCode // 变更状态码：M / A / D / R / U / C
	Staged    bool           // true 表示属于 Staged Changes，false 表示属于 Working Changes
	Untracked bool           // 是否为未跟踪新文件
}

// Key 返回该文件条目在列表和视图中的唯一标识键。
// 无输入参数，返回带分组前缀的唯一字符串标识。
func (f ChangedFile) Key() string {
	prefix := "work:"
	if f.Staged {
		prefix = "stage:"
	}
	return prefix + f.Path
}

// StatusResult 汇总一次 git status 扫描得到的全部暂存区与工作区变更。
type StatusResult struct {
	Staged   []ChangedFile
	Unstaged []ChangedFile
}

// Status 调用 git status --porcelain=v1 -z -uall 获取当前仓库的所有已暂存与未暂存文件。
// 无输入参数，返回解析后的 StatusResult 及可能的错误。
func (c *Client) Status() (StatusResult, error) {
	// 状态查询跳过可选的索引写回，避免刷新自身触发 .git 文件监听。
	out, err := c.Run("--no-optional-locks", "status", "--porcelain=v1", "-z", "-uall")
	if err != nil {
		return StatusResult{}, err
	}
	return parsePorcelainZ(out), nil
}

// parsePorcelainZ 解析 git status --porcelain=v1 -z 的 NUL 分隔输出。
// 输入参数 raw 为原始命令输出字符串，返回分类好的 StatusResult。
func parsePorcelainZ(raw string) StatusResult {
	var res StatusResult
	if raw == "" {
		return res
	}

	parts := strings.Split(raw, "\x00")
	for i := 0; i < len(parts); i++ {
		entry := parts[i]
		if len(entry) < 3 {
			continue
		}
		x := entry[0]
		y := entry[1]
		path := filepath.ToSlash(entry[3:])
		origPath := ""

		// 重命名或复制时，-z 模式下紧跟的下一个 NUL 字段是原始路径
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			if i+1 < len(parts) {
				origPath = filepath.ToSlash(parts[i+1])
				i++
			}
		}

		// 未跟踪文件
		if x == '?' && y == '?' {
			res.Unstaged = append(res.Unstaged, ChangedFile{
				Path:      path,
				Status:    StatusUntracked,
				Staged:    false,
				Untracked: true,
			})
			continue
		}

		// 冲突文件
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			res.Unstaged = append(res.Unstaged, ChangedFile{
				Path:     path,
				OrigPath: origPath,
				Status:   StatusConflict,
				Staged:   false,
			})
			continue
		}

		// 暂存区变更 (Index, X 列)
		if x != ' ' && x != '?' {
			res.Staged = append(res.Staged, ChangedFile{
				Path:     path,
				OrigPath: origPath,
				Status:   mapStatusChar(x),
				Staged:   true,
			})
		}

		// 工作区变更 (Worktree, Y 列)
		if y != ' ' && y != '?' {
			res.Unstaged = append(res.Unstaged, ChangedFile{
				Path:     path,
				OrigPath: origPath,
				Status:   mapStatusChar(y),
				Staged:   false,
			})
		}
	}

	sortFiles := func(a, b ChangedFile) int {
		return strings.Compare(a.Path, b.Path)
	}
	slices.SortFunc(res.Staged, sortFiles)
	slices.SortFunc(res.Unstaged, sortFiles)
	return res
}

// mapStatusChar 将 porcelain 单字符状态码映射为统一的 FileStatusCode。
// 输入参数 ch 为状态字节，返回对应的 FileStatusCode。
func mapStatusChar(ch byte) FileStatusCode {
	switch ch {
	case 'A':
		return StatusAdded
	case 'D':
		return StatusDeleted
	case 'R', 'C':
		return StatusRenamed
	default:
		return StatusModified
	}
}

// StageFile 将指定的文件添加到 Git 暂存区。
// 输入参数 f 为目标变更文件结构体，返回执行过程中的错误。
func (c *Client) StageFile(f ChangedFile) error {
	if f.OrigPath != "" && f.OrigPath != f.Path {
		_, err := c.Run("add", "--", f.Path, f.OrigPath)
		return err
	}
	_, err := c.Run("add", "--", f.Path)
	return err
}

// StageAll 将工作区中所有变更（包括新增、修改、删除）全部添加到暂存区。
// 无输入参数，返回执行过程中的错误。
func (c *Client) StageAll() error {
	_, err := c.Run("add", "-A")
	return err
}

// UnstageFile 将指定文件从暂存区撤回至工作区。
// 输入参数 f 为目标已暂存文件结构体，返回执行过程中的错误。
func (c *Client) UnstageFile(f ChangedFile) error {
	if c.HasHeadCommit() {
		args := []string{"reset", "HEAD", "--", f.Path}
		if f.OrigPath != "" && f.OrigPath != f.Path {
			args = append(args, f.OrigPath)
		}
		_, err := c.Run(args...)
		return err
	}
	// 尚无首次提交的空仓库使用 git rm --cached
	_, err := c.Run("rm", "--cached", "-f", "--", f.Path)
	return err
}

// UnstageAll 将所有已暂存的文件全部撤回至工作区。
// 无输入参数，返回执行过程中的错误。
func (c *Client) UnstageAll() error {
	if c.HasHeadCommit() {
		_, err := c.Run("reset", "HEAD")
		return err
	}
	_, err := c.Run("rm", "--cached", "-r", ".")
	return err
}

// DiscardFile 丢弃工作区中单个未暂存文件的修改（对于未跟踪的新文件则直接从磁盘删除）。
// 输入参数 f 为要丢弃更改的文件结构体，返回执行过程中的错误。
func (c *Client) DiscardFile(f ChangedFile) error {
	if f.Untracked {
		fullPath := filepath.Join(c.repoPath, filepath.FromSlash(f.Path))
		return os.RemoveAll(fullPath)
	}
	_, err := c.Run("checkout", "--", f.Path)
	return err
}

// DiscardAll 丢弃工作区所有未暂存的修改并清理未跟踪文件。
// 无输入参数，返回执行过程中的错误。
func (c *Client) DiscardAll() error {
	if _, err := c.Run("checkout", "--", "."); err != nil {
		return err
	}
	_, err := c.Run("clean", "-fd")
	return err
}

// HasHeadCommit 检查当前仓库是否已经存在至少一次提交（HEAD 是否有效）。
// 无输入参数，存在返回 true，空仓库返回 false。
func (c *Client) HasHeadCommit() bool {
	_, err := c.Run("rev-parse", "--verify", "HEAD")
	return err == nil
}
