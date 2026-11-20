package git

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// CodeMatchLine 表示单个文件内匹配到的一行代码结果。
type CodeMatchLine struct {
	Line    int    // 1-based 行号
	Content string // 去除过长空白后的代码行预览文本
}

// FileSearchMatch 表示包含代码匹配项的单个文件及其匹配行集合。
type FileSearchMatch struct {
	Path  string
	Lines []CodeMatchLine
}

// ProjectFileContent 封装项目单个文件的工作区最新文本、HEAD 基线文本（用于行号栏变更色条）及二进制标识。
type ProjectFileContent struct {
	Path     string
	Text     string
	HeadText string
	HasHead  bool
	IsBinary bool
}

// ListProjectFiles 列出当前仓库的所有项目文件相对路径（正斜杠分隔，已排序）。
// 输入参数 includeIgnored 为 false 时仅列出 Git 跟踪文件与未忽略的新文件（排除 .gitignore 中的垃圾目录）；
// 为 true 时遍历整个物理目录（仅排除 .git 目录）。
// 返回相对路径字符串切片和可能的错误。
func (c *Client) ListProjectFiles(includeIgnored bool) ([]string, error) {
	if !includeIgnored {
		out, err := c.Run("ls-files", "-c", "-o", "--exclude-standard", "-z")
		if err != nil {
			return nil, err
		}
		var files []string
		seen := make(map[string]bool)
		for part := range strings.SplitSeq(out, "\x00") {
			p := filepath.ToSlash(strings.TrimSpace(part))
			if p == "" || seen[p] {
				continue
			}
			// 确认文件在磁盘上仍存在（排除被删除但尚未 commit 的文件）
			full := filepath.Join(c.repoPath, filepath.FromSlash(p))
			if info, statErr := os.Stat(full); statErr == nil && !info.IsDir() {
				seen[p] = true
				files = append(files, p)
			}
		}
		slices.Sort(files)
		return files, nil
	}

	// includeIgnored == true: 扫描除 .git 外的全部物理文件
	var files []string
	err := filepath.WalkDir(c.repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(c.repoPath, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}

// ReadProjectFile 读取指定相对路径的磁盘文件内容，以及它在 HEAD 中的基线内容（若存在），供代码查看器展示与编辑。
// 输入参数 relPath 为相对于仓库根目录的路径；返回 ProjectFileContent 和错误信息。
func (c *Client) ReadProjectFile(relPath string) (ProjectFileContent, error) {
	res := ProjectFileContent{Path: relPath}
	diskText, isBin, err := c.readDiskFile(relPath)
	if err != nil {
		return res, err
	}
	if isBin {
		res.IsBinary = true
		return res, nil
	}
	res.Text = diskText

	if c.HasHeadCommit() {
		headText, headBin := c.readGitObject("HEAD:" + relPath)
		if !headBin && headText != "" {
			res.HeadText = headText
			res.HasHead = true
		}
	}
	return res, nil
}

// SearchProjectCode 在仓库中全文搜索包含 query 的代码行（基于 git grep -n -I --untracked 极速检索）。
// 输入参数 query 为搜索关键词，caseSensitive 表示是否区分大小写，includeIgnored 表示是否搜索被忽略的文件，maxMatches 为最大匹配总行数上限。
// 返回按文件分组的 FileSearchMatch 切片和可能的错误。
func (c *Client) SearchProjectCode(query string, caseSensitive bool, includeIgnored bool, maxMatches int) ([]FileSearchMatch, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if maxMatches <= 0 {
		maxMatches = 300
	}

	args := []string{"grep", "-n", "-I", "-F", "--untracked"}
	if !caseSensitive {
		args = append(args, "-i")
	}
	if includeIgnored {
		args = append(args, "--no-exclude-standard")
	} else {
		args = append(args, "--exclude-standard")
	}
	args = append(args, "-e", q)

	// git grep 在未找到任何匹配时退出码为 1，此时不属于系统错误
	out, err := c.Run(args...)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, nil
	}

	var results []FileSearchMatch
	fileIndex := make(map[string]int)
	totalLines := 0

	for rawLine := range strings.SplitSeq(out, "\n") {
		if totalLines >= maxMatches {
			break
		}
		if rawLine == "" {
			continue
		}
		// 格式: path:lineNo:content
		firstColon := strings.IndexByte(rawLine, ':')
		if firstColon <= 0 {
			continue
		}
		rest := rawLine[firstColon+1:]
		secondColon := strings.IndexByte(rest, ':')
		if secondColon <= 0 {
			continue
		}

		filePath := filepath.ToSlash(rawLine[:firstColon])
		lineNoStr := rest[:secondColon]
		lineNo, convErr := strconv.Atoi(lineNoStr)
		if convErr != nil {
			continue
		}
		content := strings.TrimSpace(rest[secondColon+1:])
		if len(content) > 400 {
			content = content[:400]
		}

		idx, exists := fileIndex[filePath]
		if !exists {
			idx = len(results)
			fileIndex[filePath] = idx
			results = append(results, FileSearchMatch{
				Path: filePath,
			})
		}
		results[idx].Lines = append(results[idx].Lines, CodeMatchLine{
			Line:    lineNo,
			Content: content,
		})
		totalLines++
	}

	return results, nil
}
