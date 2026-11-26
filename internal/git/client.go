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
	"sync"
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

// ErrGitNotFound 表示在系统 PATH 及常见安装路径中均未能找到可执行的 git 命令。
var ErrGitNotFound = errors.New("未检测到 Git，请先安装 Git 并确保 git 命令已加入系统 PATH")

// gitFallbackPaths 返回平台相关的兜底 git 可执行文件路径列表，测试时可覆盖。
var gitFallbackPaths = fallbackGitPaths

var (
	gitPathMu         sync.RWMutex
	configuredGitPath string
)

// SetGitPath 设置用户手动指定的 Git 可执行文件路径；传入空字符串时恢复自动检测。
// 输入参数 path 为目标可执行文件路径，无返回值。
func SetGitPath(path string) {
	gitPathMu.Lock()
	configuredGitPath = strings.TrimSpace(path)
	gitPathMu.Unlock()
}

// ConfiguredGitPath 返回当前已配置的自定义 Git 可执行文件路径（空字符串表示自动检测）。
// 无输入参数，返回规范化后的路径字符串。
func ConfiguredGitPath() string {
	gitPathMu.RLock()
	p := configuredGitPath
	gitPathMu.RUnlock()
	return p
}

// resolveAutoGitExecutable 在系统 PATH 及平台常见安装路径中自动查找 git 可执行文件。
// 无输入参数，返回查找到的可执行文件路径或 ErrGitNotFound。
func resolveAutoGitExecutable() (string, error) {
	if p, err := exec.LookPath("git"); err == nil {
		return p, nil
	}
	for _, p := range gitFallbackPaths() {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w", ErrGitNotFound)
}

// resolveGitExecutable 返回当前应使用的 git 可执行文件路径；优先使用用户手动配置的路径。
// 无输入参数，返回 git 可执行文件路径及可能的错误（未找到或无效时匹配 ErrGitNotFound）。
func resolveGitExecutable() (string, error) {
	if customPath := ConfiguredGitPath(); customPath != "" {
		info, err := os.Stat(customPath)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("已配置的 Git 路径无效 (%s): %w", customPath, ErrGitNotFound)
		}
		return customPath, nil
	}
	return resolveAutoGitExecutable()
}

// newCommandWithBinary 使用指定的可执行文件路径创建配置好环境变量与窗口隐藏属性的命令实例。
// 输入参数 ctx 为命令上下文，gitBin 为可执行文件路径，dir 为工作目录（可为空），args 为参数列表；返回 *exec.Cmd。
func newCommandWithBinary(ctx context.Context, gitBin, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, gitBin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	configureCommand(cmd)
	return cmd
}

// ValidateGitExecutable 校验指定路径（或空字符串触发的自动检测路径）是否为可用的 Git 命令行可执行程序。
// 输入参数 path 为待校验的路径字符串；返回解析后的完整路径、git --version 首行输出及可能的错误。
func ValidateGitExecutable(path string) (resolvedPath string, version string, err error) {
	cleaned := strings.TrimSpace(path)
	var resolved string
	if cleaned == "" {
		resolved, err = resolveAutoGitExecutable()
		if err != nil {
			return "", "", err
		}
	} else {
		cleaned = filepath.Clean(cleaned)
		info, statErr := os.Stat(cleaned)
		if statErr != nil {
			return "", "", fmt.Errorf("指定的 Git 文件不存在或无法访问: %w", statErr)
		}
		if info.IsDir() {
			return "", "", errors.New("请选择 Git 可执行文件（例如 git.exe），而不是目录")
		}
		switch strings.ToLower(filepath.Base(cleaned)) {
		case "git-bash.exe", "git-cmd.exe", "git-gui.exe", "gitk.exe",
			"sh.exe", "bash.exe", "cmd.exe", "powershell.exe", "pwsh.exe", "wt.exe":
			return "", "", errors.New(`请选择 Git 命令行程序（如 cmd\git.exe 或 bin\git.exe），而不是终端或图形包装程序`)
		}
		resolved = cleaned
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := newCommandWithBinary(ctx, resolved, "", "--version")
	out, runErr := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded) {
		return "", "", errors.New("验证 Git 可执行文件超时")
	}
	if runErr != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = runErr.Error()
		}
		return "", "", fmt.Errorf("无法运行所选 Git 文件: %s", detail)
	}

	normalized := strings.ReplaceAll(string(out), "\r\n", "\n")
	firstLine := strings.TrimSpace(strings.Split(normalized, "\n")[0])
	if !strings.Contains(strings.ToLower(firstLine), "git version") {
		return "", "", errors.New("所选文件不是有效的 Git 可执行程序")
	}
	return resolved, firstLine, nil
}

// newGitCommand 创建统一配置好的 git 子命令实例，设置工作目录、无交互环境变量及平台窗口隐藏属性。
// 输入参数 ctx 为命令上下文，dir 为工作目录，args 为 git 参数列表；返回 *exec.Cmd 及可能的错误。
func newGitCommand(ctx context.Context, dir string, args ...string) (*exec.Cmd, error) {
	gitBin, err := resolveGitExecutable()
	if err != nil {
		return nil, err
	}
	return newCommandWithBinary(ctx, gitBin, dir, args...), nil
}

// formatTopLevelError 格式化 git rev-parse --show-toplevel 失败时的错误信息，保留底层错误原因并避免出现空括号。
// 输入参数 out 为命令输出字节，err 为命令执行错误，ctxErr 为上下文错误；返回格式化后的错误。
func formatTopLevelError(out []byte, err error, ctxErr error) error {
	if errors.Is(err, ErrGitNotFound) || errors.Is(err, exec.ErrNotFound) {
		return ErrGitNotFound
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("检测 Git 仓库超时，请检查当前目录权限或 Git 环境")
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" && err != nil {
		detail = strings.TrimSpace(err.Error())
	}
	if detail == "" {
		return errors.New("当前目录不是有效的 Git 仓库")
	}
	return fmt.Errorf("当前目录不是有效的 Git 仓库 (%s)", detail)
}

// findTopLevel 调用 git rev-parse --show-toplevel 查找给定目录所属的 Git 仓库根目录。
// 输入参数 dir 为起始目录路径，返回仓库顶层绝对路径和错误信息。
func findTopLevel(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := newGitCommand(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", formatTopLevelError(out, err, ctx.Err())
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
	return c.run(stdinData, append([]string{"-c", "core.quotepath=false", "--no-pager"}, args...), args)
}

// run 在仓库目录执行 fullArgs，stdinData 可为空，label 用于错误消息；返回原始 stdout 和错误。
func (c *Client) run(stdinData string, fullArgs, label []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd, err := newGitCommand(ctx, c.repoPath, fullArgs...)
	if err != nil {
		if errors.Is(err, ErrGitNotFound) {
			return "", err
		}
		return "", fmt.Errorf("git %s 失败: %w", strings.Join(label, " "), err)
	}

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
		return "", fmt.Errorf("git %s 失败: %s", strings.Join(label, " "), errMsg)
	}
	return stdout.String(), nil
}
