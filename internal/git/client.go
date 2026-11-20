package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client 封装针对特定 Git 仓库根目录的所有底层 git CLI 调用。
type Client struct {
	repoPath string
}

// NewClient 校验给定路径是否属于一个有效的 Git 工作区，并返回对应的 Git Client。
// 输入参数 dirPath 为任意目录或文件路径；返回解析出仓库顶层目录后的 *Client 及可能的错误。
func NewClient(dirPath string) (*Client, error) {
	cleaned := strings.TrimSpace(dirPath)
	if cleaned == "" {
		return nil, errors.New("仓库路径不能为空")
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return nil, fmt.Errorf("无法访问路径 %s: %w", cleaned, err)
	}
	if !info.IsDir() {
		cleaned = filepath.Dir(cleaned)
	}

	root, err := findTopLevel(cleaned)
	if err != nil {
		return nil, err
	}
	return &Client{repoPath: root}, nil
}

// RepoPath 返回当前 Git 仓库的顶层根目录绝对路径。
// 无输入参数，返回仓库根路径字符串。
func (c *Client) RepoPath() string {
	return c.repoPath
}

// RepoName 返回当前 Git 仓库的目录名称，用于标签页标题展示。
// 无输入参数，返回目录 basename。
func (c *Client) RepoName() string {
	return filepath.Base(c.repoPath)
}

// findTopLevel 调用 git rev-parse --show-toplevel 查找给定目录所属的 Git 仓库根目录。
// 输入参数 dir 为起始目录路径，返回仓库顶层绝对路径和错误信息。
func findTopLevel(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("当前目录不是有效的 Git 仓库 (%s)", strings.TrimSpace(string(out)))
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

// Run 执行不带标准输入的 git 子命令并返回标准输出文本。
// 输入参数 args 为 git 子命令参数列表；返回 stdout 字符串和错误信息。
func (c *Client) Run(args ...string) (string, error) {
	return c.RunWithStdin("", args...)
}

// RunWithStdin 执行带标准输入内容的 git 子命令（如 git apply 从 stdin 读取 patch）。
// 输入参数 stdinData 为注入到标准输入的字符串，args 为 git 参数；返回 stdout 字符串和错误。
func (c *Client) RunWithStdin(stdinData string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 禁用 git 分页器与颜色转义码，保证解析稳定
	fullArgs := append([]string{"-c", "core.quotepath=false", "--no-pager"}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	cmd.Dir = c.repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	if stdinData != "" {
		cmd.Stdin = strings.NewReader(stdinData)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git %s 失败: %s", strings.Join(args, " "), errMsg)
	}
	return stdout.String(), nil
}
