package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// FileDiffBundle 封装单个文件用于 DiffView 渲染所需的旧版文本、新版文本、解析后的补丁以及是否为二进制等元数据。
type FileDiffBundle struct {
	File     ChangedFile
	OldText  string
	NewText  string
	Patch    *editor.FilePatch
	IsBinary bool
	Editable bool
}

// GetWorkingFileDiff 获取当前工作区或暂存区单个变更文件的完整旧版与新版文本及 git diff 补丁。
// 输入参数 f 为目标变更文件结构体；返回组装好的 FileDiffBundle 和错误信息。
func (c *Client) GetWorkingFileDiff(f ChangedFile) (FileDiffBundle, error) {
	bundle := FileDiffBundle{
		File:     f,
		Editable: !f.Staged && f.Status != StatusDeleted,
	}

	// 1. 如果是未跟踪的新文件，直接读取磁盘内容作为 NewText，OldText 为空
	if f.Untracked {
		data, isBin, err := c.readDiskFile(f.Path)
		if err != nil {
			return bundle, err
		}
		bundle.IsBinary = isBin
		bundle.NewText = data
		return bundle, nil
	}

	// 2. 获取 git diff --histogram 的补丁文本
	var diffArgs []string
	if f.Staged {
		if c.HasHeadCommit() {
			diffArgs = []string{"diff", "--cached", "--histogram", "--", f.Path}
		} else {
			// 空仓库暂存的新文件：直接从索引读取
			newText, isBin := c.readGitObject(":" + f.Path)
			bundle.IsBinary = isBin
			bundle.NewText = newText
			return bundle, nil
		}
	} else {
		diffArgs = []string{"diff", "--histogram", "--", f.Path}
	}
	if f.OrigPath != "" && f.OrigPath != f.Path {
		diffArgs = append(diffArgs, f.OrigPath)
	}

	// Diff 读取与状态查询一样跳过可选的索引写回，避免触发额外的监听刷新。
	rawDiff, _ := c.Run(append([]string{"--no-optional-locks"}, diffArgs...)...)
	if strings.TrimSpace(rawDiff) != "" {
		patches, err := editor.ParsePatch(rawDiff)
		if err == nil && len(patches) > 0 {
			p := patches[0]
			bundle.Patch = &p
			if p.Binary {
				bundle.IsBinary = true
				return bundle, nil
			}
		}
	}

	// 3. 读取旧侧文本 (OldText) 与新侧文本 (NewText)，以便 DiffView 支持完整文件展开与就地编辑
	oldPath := f.Path
	if f.OrigPath != "" {
		oldPath = f.OrigPath
	}

	if f.Staged {
		// Staged: Old 是 HEAD:<oldPath>，New 是 :<f.Path> (Index)
		if f.Status != StatusAdded {
			oldText, isBin := c.readGitObject("HEAD:" + oldPath)
			if isBin {
				bundle.IsBinary = true
				return bundle, nil
			}
			bundle.OldText = oldText
		}
		if f.Status != StatusDeleted {
			newText, isBin := c.readGitObject(":" + f.Path)
			if isBin {
				bundle.IsBinary = true
				return bundle, nil
			}
			bundle.NewText = newText
		}
	} else {
		// Unstaged: Old 是 :<oldPath> (Index)，New 是磁盘工作区文件
		if f.Status != StatusAdded {
			oldText, isBin := c.readGitObject(":" + oldPath)
			if isBin {
				bundle.IsBinary = true
				return bundle, nil
			}
			bundle.OldText = oldText
		}
		if f.Status != StatusDeleted {
			newText, isBin, err := c.readDiskFile(f.Path)
			if err == nil {
				if isBin {
					bundle.IsBinary = true
					return bundle, nil
				}
				bundle.NewText = newText
			}
		}
	}

	return bundle, nil
}

// GetCommitFileDiff 获取历史某次 Commit 中单个文件的完整前后对比内容与补丁。
// 输入参数 commitHash 为提交哈希，f 为该提交中的变更文件；返回 FileDiffBundle 及错误信息。
func (c *Client) GetCommitFileDiff(commitHash string, f ChangedFile) (FileDiffBundle, error) {
	bundle := FileDiffBundle{
		File:     f,
		Editable: false,
	}

	args := []string{"show", "--histogram", "--format=", commitHash, "--", f.Path}
	if f.OrigPath != "" && f.OrigPath != f.Path {
		args = append(args, f.OrigPath)
	}
	rawDiff, err := c.Run(args...)
	if err == nil && strings.TrimSpace(rawDiff) != "" {
		patches, parseErr := editor.ParsePatch(rawDiff)
		if parseErr == nil && len(patches) > 0 {
			p := patches[0]
			bundle.Patch = &p
			if p.Binary {
				bundle.IsBinary = true
				return bundle, nil
			}
		}
	}

	oldPath := f.Path
	if f.OrigPath != "" {
		oldPath = f.OrigPath
	}
	if f.Status != StatusAdded {
		oldText, isBin := c.readGitObject(commitHash + "~1:" + oldPath)
		if isBin {
			bundle.IsBinary = true
			return bundle, nil
		}
		bundle.OldText = oldText
	}
	if f.Status != StatusDeleted {
		newText, isBin := c.readGitObject(commitHash + ":" + f.Path)
		if isBin {
			bundle.IsBinary = true
			return bundle, nil
		}
		bundle.NewText = newText
	}
	return bundle, nil
}

// SaveWorkingFile 将用户在 DiffView 中微调编辑后的文本直接保存到工作区磁盘文件中。
// 输入参数 relPath 为相对于仓库根目录的文件路径，content 为新文本内容；返回写入错误。
func (c *Client) SaveWorkingFile(relPath string, content string) error {
	if strings.TrimSpace(relPath) == "" {
		return errors.New("文件路径不能为空")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return errors.New("非法的文件相对路径")
	}
	fullPath := filepath.Join(c.repoPath, cleanRel)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0o644)
}

// ApplySingleHunk 将指定文件的某一个 Hunk 单独应用到暂存区（Stage/Unstage Hunk）或工作区（Discard Hunk）。
// 输入参数 filePath 为文件相对路径，oldText 与 newText 为当前对比的旧新文本，hunk 为 DiffView 选中的 DiffHunk，
// cached 为 true 表示操作暂存区（--cached），reverse 为 true 表示反向撤销（-R）。
// 返回执行错误。
func (c *Client) ApplySingleHunk(filePath, oldText, newText string, hunk editor.DiffHunk, cached bool, reverse bool) error {
	patchText := buildUnifiedHunkPatch(filePath, oldText, newText, hunk)
	if patchText == "" {
		return errors.New("无法生成有效的代码块补丁")
	}

	args := []string{"apply", "--unidiff-zero", "--whitespace=nowarn"}
	if cached {
		args = append(args, "--cached")
	}
	if reverse {
		args = append(args, "-R")
	}
	_, err := c.RunWithStdin(patchText, args...)
	return err
}

// buildUnifiedHunkPatch 根据旧文本、新文本以及 DiffHunk 的行区间（0-based, [Start, End)）构造合法的零上下文 unified diff 补丁。
// 输入参数 filePath 为文件路径，oldText/newText 为旧新文本，hunk 为行范围；返回 unified diff 字符串。
func buildUnifiedHunkPatch(filePath, oldText, newText string, hunk editor.DiffHunk) string {
	oldLines := splitLinesPreserve(oldText)
	newLines := splitLinesPreserve(newText)

	oldStartIdx := max(0, min(hunk.OldStart, len(oldLines)))
	oldEndIdx := max(oldStartIdx, min(hunk.OldEnd, len(oldLines)))
	newStartIdx := max(0, min(hunk.NewStart, len(newLines)))
	newEndIdx := max(newStartIdx, min(hunk.NewEnd, len(newLines)))

	oldCount := oldEndIdx - oldStartIdx
	newCount := newEndIdx - newStartIdx
	if oldCount == 0 && newCount == 0 {
		return ""
	}

	// unified diff 1-based 行号规则：当行数为 0 时，起始行号为插入位置前一行（0-based index）
	oldHeaderStart := oldStartIdx + 1
	if oldCount == 0 {
		oldHeaderStart = oldStartIdx
	}
	newHeaderStart := newStartIdx + 1
	if newCount == 0 {
		newHeaderStart = newStartIdx
	}

	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", filePath, filePath)
	fmt.Fprintf(&b, "--- a/%s\n", filePath)
	fmt.Fprintf(&b, "+++ b/%s\n", filePath)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldHeaderStart, oldCount, newHeaderStart, newCount)

	for i := oldStartIdx; i < oldEndIdx; i++ {
		b.WriteByte('-')
		b.WriteString(oldLines[i])
		b.WriteByte('\n')
	}
	for i := newStartIdx; i < newEndIdx; i++ {
		b.WriteByte('+')
		b.WriteString(newLines[i])
		b.WriteByte('\n')
	}
	return b.String()
}

// splitLinesPreserve 将文本按 \n 切分为行切片（去除行尾 \r），空字符串返回空切片。
// 输入参数 s 为原始文本，返回按行分割的字符串切片。
func splitLinesPreserve(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// readDiskFile 读取工作区磁盘上的文件内容并检测是否为二进制文件（前 8000 字节是否含 NUL）。
// 输入参数 relPath 为相对路径；返回文本内容、是否二进制及可能发生的错误。
func (c *Client) readDiskFile(relPath string) (string, bool, error) {
	fullPath := filepath.Join(c.repoPath, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", false, err
	}
	if isBinaryBytes(data) {
		return "", true, nil
	}
	return string(data), false, nil
}

// readGitObject 调用 git show 读取指定修订或暂存区对象（如 HEAD:path 或 :path）的文本内容。
// 输入参数 spec 为 git 对象描述符；返回文本内容与是否为二进制文件的布尔值。
func (c *Client) readGitObject(spec string) (string, bool) {
	out, err := c.Run("show", spec)
	if err != nil {
		return "", false
	}
	raw := []byte(out)
	if isBinaryBytes(raw) {
		return "", true
	}
	return out, false
}

// isBinaryBytes 检查字节切片的前 8KB 是否包含 0x00 字节以判断是否为二进制文件。
// 输入参数 data 为待检测的字节切片，返回 true 表示是二进制数据。
func isBinaryBytes(data []byte) bool {
	checkLen := min(len(data), 8000)
	return bytes.IndexByte(data[:checkLen], 0) != -1
}
