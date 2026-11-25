package app

import (
	"context"
	"encoding/json"
	"fmt"
	"go/version"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"
)

const (
	releaseAPI  = "https://api.github.com/repos/dannywsh/git-me/releases/latest"
	releasesURL = "https://github.com/dannywsh/git-me/releases"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type updateControls struct {
	selfInstall bool
	checking    bool
	ctx         context.Context
	window      *mygo.Window
}

// ConfigureUpdates 在应用启动前初始化更新状态，安装目录可写时注册签名更新插件。
// 无输入参数，无返回值。
func (a *App) ConfigureUpdates() {
	a.updates = &updateControls{selfInstall: !linuxPackageInstall() && mygo.Updater.Enabled()}
	if a.updates.selfInstall {
		mygo.Use(native.Plugin)
	}
}

// linuxPackageInstall 判断当前程序是否由 Linux 系统软件包安装在 /opt/git-me。
// 无输入参数，返回是否应交由包管理器安装更新。
func linuxPackageInstall() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe) == "/opt/git-me"
}

// startUpdateChecks 在无法自动安装的发行版中启动一次后台检查，并在主窗口关闭时取消。
// 输入参数 win 为主窗口，无返回值。
func (a *App) startUpdateChecks(win *mygo.Window) {
	u := a.updates
	if u == nil || u.selfInstall {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.ctx, u.window = ctx, win
	win.OnClosed(cancel)
	if a.Settings.CheckUpdatesOnLaunch && !mygo.IsDev() && mygo.App.Version() != "" {
		u.checkRelease(false)
	}
}

// checkRelease 在后台查询 GitHub 的正式版本，必要时向用户显示结果。
// 输入参数 manual 表示是否由用户触发，无返回值。
func (u *updateControls) checkRelease(manual bool) {
	if u.checking || u.ctx == nil || u.ctx.Err() != nil {
		return
	}
	u.checking = true
	window, windowCtx := u.window, u.ctx
	current := mygo.App.Version()
	go func() {
		defer window.Update(func() { u.checking = false })
		if current == "" {
			_ = mygo.Shell.OpenExternal(releasesURL)
			return
		}
		ctx, cancel := context.WithTimeout(windowCtx, 12*time.Second)
		tag, err := latestRelease(ctx, http.DefaultClient, releaseAPI, current)
		cancel()
		if windowCtx.Err() != nil || (!manual && tag == "") {
			return
		}

		dialog := mygo.MessageOptions{Parent: window, Title: "Gitme 软件更新"}
		switch {
		case err != nil:
			dialog.Message = "检查更新失败"
			dialog.Detail = err.Error()
		case tag == "":
			dialog.Message = "Gitme 已是最新版本"
			dialog.Detail = "当前版本 " + current
		default:
			dialog.Message = "Gitme " + tag + " 已发布"
			dialog.Detail = "请从 GitHub Releases 下载并安装适合当前系统的新版。"
			dialog.Buttons = []string{"打开发布页面", "稍后"}
			dialog.CancelButton = 1
		}
		result, err := mygo.Dialog.Message(dialog)
		if err == nil && tag != "" && result.Button == 0 && windowCtx.Err() == nil {
			_ = mygo.Shell.OpenExternal(releasesURL + "/tag/" + tag)
		}
	}()
}

// latestRelease 使用 client 从 endpoint 查询正式发行版，与 current 比较后返回较新的标签或错误。
// 返回空标签且无错误表示没有可用更新。
func latestRelease(ctx context.Context, client *http.Client, endpoint, current string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", err
	}
	if release.Draft || release.Prerelease {
		return "", nil
	}
	latest, tagged := strings.CutPrefix(release.Tag, "v")
	current, _, _ = strings.Cut(current, "+")
	base, _, prerelease := strings.Cut(current, "-")
	if !tagged || !stableVersion.MatchString(latest) || !stableVersion.MatchString(base) {
		return "", fmt.Errorf("无法比较版本 %q 和 %q", current, release.Tag)
	}
	cmp := version.Compare("go"+latest, "go"+base)
	if cmp > 0 || (cmp == 0 && prerelease) {
		return release.Tag, nil
	}
	return "", nil
}
