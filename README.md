<p align="center">
  <img src="resources/icon.svg" width="96" alt="Gitme 图标">
</p>

# Gitme

Gitme 是一款轻量级 Git 图形界面工具。它使用 [MyGo](https://github.com/egoist/mygo) 构建原生桌面界面，适合在提交前检查代码变更，也能查看历史提交和浏览项目文件。

## 功能

- **多仓库标签页**。通过目录选择器、最近打开列表、拖放文件夹或命令行打开仓库。
- **最近仓库**。点击标签栏末尾的「＋」选择目录。在左侧文件侧栏底部展开「最近仓库」，点击记录打开仓库，或悬停后点击右侧「×」移除该记录。移除记录时，已打开的标签页会继续保留。
- **仓库详情**。顶栏右侧显示当前仓库的 Fetch 地址，优先显示 `origin`。点击信息图标后，默认查看按用途分组的本项目 Git 配置，包括仓库和当前工作树的设置。「全局」显示用户级配置，「其他来源」在存在系统或命令行配置时出现。每项配置都标明来源并隐藏常见凭据值。「仓库概览」汇集工作区、Git 目录、分支、HEAD 和 Fetch / Push 地址。
- **变更审查**。按已暂存和未暂存分组查看文件，在列表与目录树之间切换。Diff 支持左右分栏、上下合并，以及逐个文件或连续浏览。
- **暂存和提交**。可以按文件暂存、撤回暂存或放弃修改，也可以按代码块处理。支持提交、修正最近一次提交、创建和切换分支，以及 Fetch、Pull 和 Push。
- **提交历史和项目搜索**。查看单次提交的文件与 Diff，按文件名或内容搜索，点击匹配结果定位到对应行。
- **自动刷新**。监听工作区和 Git 状态变化，外部修改后自动更新界面，并保护尚未保存的 Diff 编辑。工具栏显示最近一次成功刷新的时间，一小时内显示相对时间，之后显示本地日期和时间。
- **自定义 Git 路径与程序设置**。通过系统菜单栏（macOS「Gitme → 设置…」，Windows 与 Linux「文件 → 设置…」）或快捷键打开原生设置窗口，集中管理通用偏好以及手动指定 `git` 可执行文件（如 Windows 下的 `git.exe`），在 Git 未加入 `PATH` 或注册表时也能正常使用。
- **软件更新**。支持手动检查、启动检查和自动安装更新。

## 快速开始

需要 Go 1.27.4 或更高版本，以及可从命令行调用的 Git。在 Gitme 的源码目录中运行：

```sh
go tool mygo dev
```

也可以直接启动并打开指定的仓库。将示例路径替换成你自己的仓库路径。

```sh
go run . /path/to/repo
```

macOS 上可以用 `⌘O` 打开仓库、`⌘R` 刷新状态、`⌘S` 保存编辑、`⌘,` 打开设置（Windows 与 Linux 对应 `Ctrl` 组合键）。窗口标题显示 Gitme。原生菜单支持简体中文、繁体中文、日语和英语，默认匹配系统首选语言，也可在 macOS「语言与地区」中为应用单独设置。

## 软件更新

macOS 在屏幕顶部选择「Gitme → 检查更新…」。Windows 和 Linux 在主窗口选择「帮助 → 检查更新…」。在同一菜单的「更新设置」中可以调整启动检查和自动安装选项。自动安装的更新会在下次启动时生效。

使用 Linux DEB 或 RPM 安装的用户，可以从发布页下载新版并按原方式安装。从 0.1.0 升级时，请先手动下载安装新版。

## 测试与打包

```sh
go test ./...
go tool mygo vet .
go tool mygo build
```

构建产物位于各平台对应的 `build/` 子目录。例如，Apple 芯片 Mac 的应用包为 `build/darwin-arm64/Gitme.app`。

## 安装

从 [GitHub Releases](https://github.com/dannywsh/git-me/releases/latest) 下载适合当前系统的安装包。运行 Gitme 前请先安装 Git。

- **macOS**。下载通用 DMG，打开后将 Gitme 拖进「应用程序」。支持 Apple 芯片和 Intel Mac。
- **Windows**。按系统架构选择 x64 或 ARM64 安装包。
- **Linux**。按发行版和系统架构选择 DEB、RPM 或 TAR.GZ。

## 项目结构

| 路径 | 用途 |
| --- | --- |
| [main.go](main.go) | 应用入口和窗口初始化 |
| [internal/app](internal/app) | 仓库标签页、Diff 界面和异步刷新 |
| [internal/git](internal/git) | Git 命令、状态解析、Diff 和搜索 |
| [internal/watcher](internal/watcher) | 工作区和 Git 状态监听 |
| [internal/config](internal/config) | 本地设置和最近打开的仓库 |
| [locales](locales) | 原生菜单语言配置和应用介绍 |
