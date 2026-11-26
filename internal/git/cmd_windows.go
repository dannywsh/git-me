//go:build windows

package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// createNoWindow 对应 Win32 CREATE_NO_WINDOW 标志，防止 GUI 应用启动 git 时在 Windows 11 弹出 Windows Terminal 或控制台窗口。
const createNoWindow = 0x08000000

// configureCommand 为 Windows 平台上的 git 子进程配置隐藏控制台窗口属性。
// 输入参数 cmd 为待执行的 *exec.Cmd；无返回值。
func configureCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// fallbackGitPaths 返回 Windows 上常见的 Git 可执行文件安装路径列表（已去重），用于在 PATH 缺失 git 时兜底查找。
// 无输入参数，返回候选 git.exe 绝对路径切片。
func fallbackGitPaths() []string {
	candidates := []struct {
		envKey   string
		subPaths []string
	}{
		{"ProgramFiles", []string{"Git", "cmd", "git.exe"}},
		{"ProgramW6432", []string{"Git", "cmd", "git.exe"}},
		{"ProgramFiles(x86)", []string{"Git", "cmd", "git.exe"}},
		{"LocalAppData", []string{"Programs", "Git", "cmd", "git.exe"}},
	}

	seen := make(map[string]bool, len(candidates))
	var paths []string
	for _, c := range candidates {
		base := os.Getenv(c.envKey)
		if base == "" {
			continue
		}
		p := filepath.Join(append([]string{base}, c.subPaths...)...)
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}
