<p align="center">
  <img src="resources/icon.svg" width="96" alt="Gitme 图标">
</p>

# Gitme

Gitme 是一款轻量级 Git 图形界面工具。它使用 [MyGo](https://github.com/egoist/mygo) 构建原生桌面界面，适合在提交前检查代码变更，也能查看历史提交和浏览项目文件。

## 功能

- **多仓库标签页**。通过目录选择器、最近打开列表、拖放文件夹或命令行打开仓库。
- **变更审查**。按已暂存和未暂存分组查看文件，在列表与目录树之间切换。Diff 支持左右分栏、上下合并，以及逐个文件或连续浏览。
- **暂存和提交**。可以按文件暂存、撤回暂存或放弃修改，也可以按代码块处理。支持提交、修正最近一次提交、创建和切换分支，以及 Fetch、Pull 和 Push。
- **提交历史和项目搜索**。查看单次提交的文件与 Diff，按文件名或内容搜索，点击匹配结果定位到对应行。
- **自动刷新**。监听工作区和 Git 状态变化，外部修改后自动更新界面，并保护尚未保存的 Diff 编辑。工具栏显示最近一次成功刷新的时间，一小时内显示相对时间，之后显示本地日期和时间。

## 快速开始

需要 Go 1.27.4 或更高版本，以及可从命令行调用的 Git。在 Gitme 的源码目录中运行：

```sh
go tool mygo dev
```

也可以直接启动并打开指定的仓库。将示例路径替换成你自己的仓库路径。

```sh
go run . /path/to/repo
```

macOS 上可以用 `⌘O` 打开仓库、`⌘R` 刷新状态、`⌘S` 保存编辑。窗口标题显示 Gitme。原生菜单支持简体中文、繁体中文、日语和英语，默认匹配系统首选语言，也可在 macOS「语言与地区」中为应用单独设置。

## 测试与打包

```sh
go test ./...
go tool mygo vet .
go tool mygo build
```

构建产物位于各平台对应的 `build/` 子目录。例如，Apple 芯片 Mac 的应用包为 `build/darwin-arm64/Gitme.app`。

## 项目结构

| 路径 | 用途 |
| --- | --- |
| [main.go](main.go) | 应用入口和窗口初始化 |
| [internal/app](internal/app) | 仓库标签页、Diff 界面和异步刷新 |
| [internal/git](internal/git) | Git 命令、状态解析、Diff 和搜索 |
| [internal/watcher](internal/watcher) | 工作区和 Git 状态监听 |
| [internal/config](internal/config) | 本地设置和最近打开的仓库 |
| [locales](locales) | 原生菜单语言配置和应用介绍 |
