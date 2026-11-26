# Changelog

## v0.1.3

### Enhancements
- (settings) 新增原生设置窗口，支持手动选择 Git 可执行文件路径与通用偏好设置
- (menu) 在系统菜单栏新增本地化「设置…」入口，并完善 Windows / Linux 原生菜单栏

### Bug Fixes
- (windows) 修复调用 Git 子进程时频繁弹出 Windows Terminal / 控制台窗口的问题
- (windows) 修复 Git 未加入 PATH 时的常见安装路径兜底查找及 CRLF 远程地址解析问题
- (app) 修复非 Git 目录启动时欢迎页误报空括号错误 `当前目录不是有效的 Git 仓库 ()` 的问题
