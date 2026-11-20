package git

import (
	"path/filepath"
	"slices"
	"strings"
)

// CommitEntry 描述提交历史列表中的单次 Git Commit 元数据。
type CommitEntry struct {
	Hash      string
	ShortHash string
	Subject   string
	Author    string
	RelTime   string
}

// RecentCommits 查询当前分支最近的 maxCount 条提交记录。
// 输入参数 maxCount 为最大返回条数（<=0 时默认 50 条）；返回 CommitEntry 列表和可能的错误。
func (c *Client) RecentCommits(maxCount int) ([]CommitEntry, error) {
	if maxCount <= 0 {
		maxCount = 50
	}
	if !c.HasHeadCommit() {
		return nil, nil
	}

	out, err := c.Run(
		"log",
		"-n", strconvItoa(maxCount),
		"--format=%H%x1f%h%x1f%s%x1f%an%x1f%cr%x1e",
	)
	if err != nil {
		return nil, err
	}

	var list []CommitEntry
	for rec := range strings.SplitSeq(out, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		fields := strings.Split(rec, "\x1f")
		if len(fields) < 5 {
			continue
		}
		list = append(list, CommitEntry{
			Hash:      fields[0],
			ShortHash: fields[1],
			Subject:   fields[2],
			Author:    fields[3],
			RelTime:   fields[4],
		})
	}
	return list, nil
}

// CommitFiles 查询指定 Commit 中修改的所有文件列表及其变更状态。
// 输入参数 commitHash 为目标提交哈希；返回 ChangedFile 切片及可能的错误。
func (c *Client) CommitFiles(commitHash string) ([]ChangedFile, error) {
	if strings.TrimSpace(commitHash) == "" {
		return nil, nil
	}
	out, err := c.Run("diff-tree", "--no-commit-id", "--name-status", "-r", "-z", "--root", commitHash)
	if err != nil {
		return nil, err
	}
	return parseNameStatusZ(out), nil
}

// parseNameStatusZ 解析 git diff-tree --name-status -z 的 NUL 分隔输出。
// 输入参数 raw 为原始命令输出，返回解析后的 ChangedFile 切片。
func parseNameStatusZ(raw string) []ChangedFile {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\x00")
	var files []ChangedFile
	for i := 0; i < len(parts); i++ {
		statusToken := strings.TrimSpace(parts[i])
		if statusToken == "" || i+1 >= len(parts) {
			continue
		}
		codeChar := statusToken[0]
		firstPath := filepath.ToSlash(parts[i+1])
		i++

		if codeChar == 'R' || codeChar == 'C' {
			if i+1 < len(parts) {
				newPath := filepath.ToSlash(parts[i+1])
				i++
				files = append(files, ChangedFile{
					Path:     newPath,
					OrigPath: firstPath,
					Status:   StatusRenamed,
					Staged:   true,
				})
			}
			continue
		}

		files = append(files, ChangedFile{
			Path:   firstPath,
			Status: mapStatusChar(codeChar),
			Staged: true,
		})
	}

	slices.SortFunc(files, func(a, b ChangedFile) int {
		return strings.Compare(a.Path, b.Path)
	})
	return files
}

// strconvItoa 将正整数转换为十进制字符串。
// 输入参数 n 为整数，返回对应的十进制字符串。
func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
