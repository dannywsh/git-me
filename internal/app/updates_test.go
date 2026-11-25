package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"git-me/internal/config"

	"github.com/egoist/mygo"
)

// TestLatestRelease 用本地 HTTP 响应验证正式版本的比较、过滤和错误处理。
// t 为当前测试，无返回值。
func TestLatestRelease(t *testing.T) {
	cases := []struct {
		name, current, body, want string
		status                    int
		wantErr                   bool
	}{
		{name: "newer", current: "0.9.0", body: `{"tag_name":"v0.10.0"}`, want: "v0.10.0"},
		{name: "same", current: "0.10.0+local", body: `{"tag_name":"v0.10.0"}`},
		{name: "older", current: "1.0.0", body: `{"tag_name":"v0.10.0"}`},
		{name: "promoted", current: "0.10.0-rc.1", body: `{"tag_name":"v0.10.0"}`, want: "v0.10.0"},
		{name: "prerelease", current: "0.9.0", body: `{"tag_name":"v0.10.0-rc.1","prerelease":true}`},
		{name: "draft", current: "0.9.0", body: `{"tag_name":"v0.10.0","draft":true}`},
		{name: "invalid tag", current: "0.9.0", body: `{"tag_name":"v0.10.0/../other"}`, wantErr: true},
		{name: "no releases", current: "0.9.0", status: http.StatusNotFound},
		{name: "rate limited", current: "0.9.0", status: http.StatusTooManyRequests, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			tag, err := latestRelease(t.Context(), server.Client(), server.URL, tc.current)
			if tag != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("latestRelease = %q, %v; want %q, error=%t", tag, err, tc.want, tc.wantErr)
			}
		})
	}
}

// TestUpdateMenuPersistsStartupPreference 验证更新菜单可切换并保存启动检查偏好。
// t 为当前测试，无返回值。
func TestUpdateMenuPersistsStartupPreference(t *testing.T) {
	t.Setenv("MYGO_ENV", "production")
	a := &App{
		Store:    config.NewStore(t.TempDir()),
		Settings: config.DefaultSettings(),
		updates:  &updateControls{},
	}
	item := a.updateMenu("darwin", "zh-Hans").ItemByID(checkAtStartupID)
	if item == nil || !item.IsChecked() || !item.IsEnabled() || item.Type != mygo.MenuItemCheckbox {
		t.Fatalf("missing enabled startup checkbox: %+v", item)
	}
	item.SetChecked(false) // 原生菜单先切换复选状态，再调用回调。
	item.Click(item, nil)
	saved, err := a.Store.Load()
	if err != nil || saved.CheckUpdatesOnLaunch {
		t.Fatalf("startup preference not disabled: %+v, %v", saved, err)
	}
}

// TestUpdateMenuPlacement 验证 macOS 应用菜单和 Windows 帮助菜单包含本地化的检查命令。
// t 为当前测试，无返回值。
func TestUpdateMenuPlacement(t *testing.T) {
	a := &App{Settings: config.DefaultSettings(), updates: &updateControls{}}
	for _, tc := range []struct {
		goos, locale, label string
		role                mygo.MenuRole
		index               int
	}{
		{"darwin", "zh-Hans", "检查更新…", mygo.RoleAppMenu, 2},
		{"windows", "en", "Check for Updates…", mygo.RoleHelp, 0},
		{"linux", "zh-Hant", "檢查更新…", mygo.RoleHelp, 0},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			menu := a.updateMenu(tc.goos, tc.locale).Items()[0]
			if menu.Role != tc.role || len(menu.Submenu) <= tc.index {
				t.Fatalf("unexpected menu: %+v", menu)
			}
			check := menu.Submenu[tc.index]
			if check.Label != tc.label || check.Click == nil {
				t.Fatalf("unexpected update action: %+v", check)
			}
		})
	}
}
