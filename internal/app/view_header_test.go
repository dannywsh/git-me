package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// TestRepoToolbarRefreshTime 验证工具栏的刷新时间随时间变化。t 为当前测试，无返回值。
func TestRepoToolbarRefreshTime(t *testing.T) {
	tab := &RepoTab{}
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		tab.viewRepoToolbar(c)
	}, 800, 80)

	tab.lastRefreshAt = tt.Now()
	tt.Frame()
	if !tt.HasText("刚刚") {
		t.Fatalf("missing recent refresh time: %v", tt.Texts())
	}
	tt.Advance(10 * time.Second)
	if !tt.HasText("10 秒前") {
		t.Fatalf("missing refresh age in seconds: %v", tt.Texts())
	}
	tt.Advance(50 * time.Second)
	if !tt.HasText("1 分钟前") {
		t.Fatalf("missing refresh age in minutes: %v", tt.Texts())
	}
	tt.Advance(59 * time.Minute)
	if want := tab.lastRefreshAt.Local().Format("2006-01-02 15:04"); !tt.HasText(want) {
		t.Fatalf("missing absolute refresh time %q: %v", want, tt.Texts())
	}
}
