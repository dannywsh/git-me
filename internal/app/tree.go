package app

import (
	"slices"
	"strings"

	"git-me/internal/git"
)

// TreeNode 表示目录树视图中的一个节点（目录或叶节点文件）。
type TreeNode struct {
	Name     string
	FullPath string
	IsDir    bool
	File     git.ChangedFile
	Children []*TreeNode
}

// BuildFileTree 将平铺的 ChangedFile 列表转换为经过单子节点目录折叠优化（Compact Folder）的层级目录树，根节点包裹为 repoName。
// 输入参数 repoName 为仓库根目录显示名称（为空时不包裹顶层根节点），files 为平铺变更文件列表，filter 为搜索过滤词（不区分大小写）。
// 返回目录树节点切片。
func BuildFileTree(repoName string, files []git.ChangedFile, filter string) []*TreeNode {
	root := &TreeNode{
		Name:     repoName,
		FullPath: "/",
		IsDir:    true,
	}
	q := strings.ToLower(strings.TrimSpace(filter))

	for _, f := range files {
		if q != "" && !strings.Contains(strings.ToLower(f.Path), q) {
			continue
		}
		insertIntoTree(root, f)
	}

	compactAndSortTree(root)
	for len(root.Children) == 1 && root.Children[0].IsDir {
		only := root.Children[0]
		if root.Name != "" {
			root.Name = root.Name + "/" + only.Name
		} else {
			root.Name = only.Name
		}
		root.FullPath = only.FullPath
		root.Children = only.Children
	}
	if root.Name != "" && len(root.Children) > 0 {
		return []*TreeNode{root}
	}
	return root.Children
}

// BuildProjectPathTree 将项目所有文件的相对路径列表转换为层级目录树，并标记哪些文件在当前工作区有修改。
// 输入参数 repoName 为仓库根目录显示名称，paths 为项目文件相对路径切片，statusMap 为当前变更文件的状态映射（path -> FileStatusCode）；
// 返回目录树节点切片。
func BuildProjectPathTree(repoName string, paths []string, statusMap map[string]git.FileStatusCode) []*TreeNode {
	root := &TreeNode{
		Name:     repoName,
		FullPath: "/",
		IsDir:    true,
	}
	for _, p := range paths {
		code := statusMap[p]
		insertIntoTree(root, git.ChangedFile{
			Path:   p,
			Status: code,
		})
	}
	compactAndSortTree(root)
	for len(root.Children) == 1 && root.Children[0].IsDir {
		only := root.Children[0]
		if root.Name != "" {
			root.Name = root.Name + "/" + only.Name
		} else {
			root.Name = only.Name
		}
		root.FullPath = only.FullPath
		root.Children = only.Children
	}
	if root.Name != "" && len(root.Children) > 0 {
		return []*TreeNode{root}
	}
	return root.Children
}

// insertIntoTree 将单个文件条目按斜杠层级插入到目录树根节点下。
// 输入参数 root 为树根节点，f 为文件条目；无返回值。
func insertIntoTree(root *TreeNode, f git.ChangedFile) {
	parts := strings.Split(f.Path, "/")
	curr := root
	var pathSoFar strings.Builder

	for i, part := range parts {
		if i > 0 {
			pathSoFar.WriteByte('/')
		}
		pathSoFar.WriteString(part)
		curPath := pathSoFar.String()
		isLast := i == len(parts)-1

		if isLast {
			curr.Children = append(curr.Children, &TreeNode{
				Name:     part,
				FullPath: curPath,
				IsDir:    false,
				File:     f,
			})
		} else {
			var found *TreeNode
			for _, child := range curr.Children {
				if child.IsDir && child.Name == part {
					found = child
					break
				}
			}
			if found == nil {
				found = &TreeNode{
					Name:     part,
					FullPath: curPath,
					IsDir:    true,
				}
				curr.Children = append(curr.Children, found)
			}
			curr = found
		}
	}
}

// FilterFiles 根据搜索关键字过滤平铺的 ChangedFile 列表。
// 输入参数 files 为原始文件切片，filter 为搜索关键字；返回匹配的文件切片。
func FilterFiles(files []git.ChangedFile, filter string) []git.ChangedFile {
	q := strings.ToLower(strings.TrimSpace(filter))
	if q == "" {
		return files
	}
	var out []git.ChangedFile
	for _, f := range files {
		if strings.Contains(strings.ToLower(f.Path), q) {
			out = append(out, f)
		}
	}
	return out
}

// FilterPaths 根据搜索关键字和大小写选项过滤项目文件路径列表。
// 输入参数 paths 为原始路径切片，query 为搜索词，caseSensitive 表示是否区分大小写；返回匹配的路径切片。
func FilterPaths(paths []string, query string, caseSensitive bool) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return paths
	}
	if !caseSensitive {
		q = strings.ToLower(q)
	}
	var out []string
	for _, p := range paths {
		target := p
		if !caseSensitive {
			target = strings.ToLower(p)
		}
		if strings.Contains(target, q) {
			out = append(out, p)
		}
	}
	return out
}

// compactAndSortTree 递归对目录树进行排序（文件夹在前、文件在后）并合并单子目录路径（如 internal/git）。
// 输入参数 node 为当前处理的目录树节点，无返回值。
func compactAndSortTree(node *TreeNode) {
	for i, child := range node.Children {
		if !child.IsDir {
			continue
		}
		compactAndSortTree(child)
		for len(child.Children) == 1 && child.Children[0].IsDir {
			only := child.Children[0]
			child.Name = child.Name + "/" + only.Name
			child.FullPath = only.FullPath
			child.Children = only.Children
			node.Children[i] = child
		}
	}

	slices.SortFunc(node.Children, func(a, b *TreeNode) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
}
