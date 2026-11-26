//go:build !windows

package git

import "os/exec"

// configureCommand 在非 Windows 平台上无需配置隐藏控制台窗口属性。
// 输入参数 cmd 为待执行的 *exec.Cmd；无返回值。
func configureCommand(cmd *exec.Cmd) {}

// fallbackGitPaths 在非 Windows 平台上不提供额外兜底路径，依赖系统 PATH 查找 git。
// 无输入参数，返回 nil。
func fallbackGitPaths() []string {
	return nil
}
