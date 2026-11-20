package watcher

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// RepoWatcher 监听指定 Git 仓库目录的文件与 .git 状态变化，并通过防抖触发回调。
type RepoWatcher struct {
	repoPath string
	onChange func()
	mu       sync.Mutex
	cancel   context.CancelFunc
	fw       *mygo.FileWatcher
}

// NewRepoWatcher 创建并启动一个针对 repoPath 的防抖文件监听器。
// 输入参数 repoPath 为仓库根目录绝对路径，onChange 为检测到变更稳定后的回调函数。
// 返回 *RepoWatcher 实例。
func NewRepoWatcher(repoPath string, onChange func()) *RepoWatcher {
	w := &RepoWatcher{
		repoPath: repoPath,
		onChange: onChange,
	}
	w.start()
	return w
}

// start 启动底层的 mygo.WatchFiles 协程，监听工作区与 .git 关键引用变化。
// 无输入参数，无返回值。
func (w *RepoWatcher) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	go func() {
		fw, err := mygo.WatchFiles(ctx, w.repoPath, mygo.FileWatchOptions{
			Recursive: true,
			HintsOnly: true,
			Debounce:  300 * time.Millisecond,
			MaxDelay:  1200 * time.Millisecond,
			Ignore:    shouldIgnoreWatchDir,
		})
		if err != nil {
			return
		}
		w.mu.Lock()
		w.fw = fw
		w.mu.Unlock()
		defer fw.Close()

		for {
			events, err := fw.Next(ctx)
			if err != nil {
				return
			}
			if len(events) > 0 && w.onChange != nil {
				w.onChange()
			}
		}
	}()
}

// shouldIgnoreWatchDir 判断目录是否应当被文件监听器跳过，以避免海量依赖目录与 .git/objects 临时锁导致高频误触发。
// 输入参数 relPath 为相对于仓库根目录的正斜杠路径，isDir 表示是否为目录。
// 返回 true 表示忽略该目录。
func shouldIgnoreWatchDir(relPath string, isDir bool) bool {
	if !isDir {
		return false
	}
	// 忽略常见大型构建产物和依赖目录
	switch relPath {
	case "node_modules", "vendor", "target", "dist", "build", ".mygo", ".next", ".venv", "venv", "__pycache__":
		return true
	}
	if strings.HasPrefix(relPath, "node_modules/") ||
		strings.HasPrefix(relPath, "target/") ||
		strings.HasPrefix(relPath, ".mygo/") ||
		strings.HasPrefix(relPath, ".venv/") {
		return true
	}
	// 保留 .git 根目录及 .git/refs 以感知外部 git commit / branch / stage 变化，但忽略 .git/objects 与 .git/logs
	if relPath == ".git/objects" || strings.HasPrefix(relPath, ".git/objects/") ||
		relPath == ".git/logs" || strings.HasPrefix(relPath, ".git/logs/") ||
		relPath == ".git/hooks" || strings.HasPrefix(relPath, ".git/hooks/") {
		return true
	}
	return false
}

// Stop 停止当前仓库的文件监听并释放系统句柄。
// 无输入参数，无返回值。
func (w *RepoWatcher) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	fw := w.fw
	w.cancel = nil
	w.fw = nil
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if fw != nil {
		_ = fw.Close()
	}
}
