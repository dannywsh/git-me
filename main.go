package main

import (
	"log"
	"os"

	"git-me/internal/app"
	_ "git-me/locales" // 启用系统语言匹配和原生菜单翻译。

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// main 是 Gitme 桌面应用程序的启动入口，解析命令行参数并创建 MyGo 原生 UI 主窗口。
// 无输入参数，无返回值。
func main() {
	var initialPaths []string
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			if len(arg) > 0 && arg[0] != '-' {
				initialPaths = append(initialPaths, arg)
			}
		}
	}

	application := app.NewApp(initialPaths)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Gitme",
			Width:     1280,
			Height:    820,
			MinWidth:  860,
			MinHeight: 540,
			StateKey:  "git-me-main",
			Content:   ui.View(application.View),
		})
		application.SetWindow(win)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
