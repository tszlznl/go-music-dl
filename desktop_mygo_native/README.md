# Go Music DL - Native UI (Pure Go GPU 渲染原生桌面端)

基于 [egoist/mygo](https://github.com/egoist/mygo) 原生 UI 引擎（`github.com/egoist/mygo/ui`）构建的跨平台音乐搜索、试听与下载桌面客户端。

## 🌟 核心特色

1. **纯 Go 编写，零 CGO 依赖**：
   - 跨平台编译无需安装 gcc、clang 等 C 语言工具链，即开即用。
2. **纯 GPU 原生声明式渲染**：
   - 无 Webview、无 Chromium、无 HTML/CSS/JS，内存占用极低（通常只需 20~40MB），启动速度毫秒级，真 60FPS 流畅动效。
3. **Pulse 沉浸式设计系统**：
   - **明暗双模主题**：Pulse Dark（#0b0b0d 深邃黑）与 Pulse Light（#f5f5f7 极简白），支持标题栏微按钮与设置页深度切换。
   - **多平台窗口控制**：支持系统自适应、Windows (WinUI 3 贴边按钮)、macOS (红黄绿交通灯) 以及 Linux GNOME / KDE 风格无缝体验。
   - **矢量图标与字体系统**：集成 Lucide 精致矢量图标；支持精选字体（微软雅黑、Segoe UI、苹方、SF Pro 等）与本机字体实时字形预览。
4. **内置纯 Go 音频解码播放**：
   - 基于 `github.com/egoist/mygo/audio` 与 `github.com/egoist/mygo/audio/mp3`，实现纯 Go MP3 音频流式解码与系统扬声器输出，支持音量调节、暂停/恢复、可拖拽 Seek 进度条与循环播放。
5. **全网多平台音乐聚合搜索**：
   - 支持网易云音乐、QQ音乐、酷狗音乐、酷我音乐、咪咕音乐、Bilibili、汽水音乐等主流音源并发搜索，支持单曲、歌单、专辑与链接直达。
6. **歌单与本地音乐管理**：
   - **推荐与解析**：支持官方推荐歌单、歌单详情解析与一键播放全部。
   - **自制歌单与收藏**：支持创建本地自制歌单、单曲收藏，自动持久化至本地 `collections.json`。
   - **本地音乐扫描**：离线扫描下载目录的音频文件，支持本地歌曲离线播放与格式防御。
   - **LRC 实时同步歌词**：浮动悬停歌词弹窗，随播放进度实时毫秒级精准追踪。
7. **一键扫码授权与真实身份同步**：
   - 支持网易云、QQ音乐、酷狗、B站一键扫码自动捕获 Cookie，轮询检测手机端授权，自动同步登录账号昵称与 VIP 身份。
   - 支持剪贴板智能识别一键写入多平台 Cookie。
8. **完整下载管理与配置持久化**：
   - **音质选择**：支持无损优先 (FLAC)、极高 (320Kbps) 与标准 (128Kbps) 音质偏好选择。
   - **系统原生交互**：系统原生目录选择对话框（`mygo.Dialog.Open`）、在资源管理器中定位（`mygo.Shell.ShowItemInFolder`）、系统桌面通知推送（`mygo.NewNotification`）。
   - **单实例运行保护**：`mygo.App.RequestSingleInstanceLock`。
   - **本地设置落盘**：所有主题、窗口风格、字体、音量与下载偏好自动持久化至 `data/desktop_settings.json`，重启自动还原。

## 📁 目录结构

```text
desktop_mygo_native/
├── main.go             # 原生 UI 客户端主体逻辑（侧边栏、搜索、推荐、本地音乐、收藏歌单、下载管理、设置、底部播放条、扫码弹窗）
├── main_test.go        # 基于 ui.Tester 的自动化测试套件（18项全功能覆盖）
├── icons.go            # Lucide 精细矢量 SVG 图标定义库
├── icon.png            # 嵌入式 256x256 高清应用图标资源
├── app.desktop         # Linux 桌面快捷方式配置
└── README.md           # 原生桌面客户端说明文档
```

## 🚀 快速运行与打包

### 本地开发启动

Windows:
```bat
run_mygo_native.bat
```

Linux / macOS:
```bash
./run_mygo_native.sh
```

### 生产环境打包

Windows（生成带嵌入图标、Windows 清单且无终端黑框的独立可执行文件）：
```bat
package_mygo_native.bat
```
生成产物：`music-dl-desktop-mygo-native.exe`（直接双击即可运行，体积约 21MB）。

Linux / macOS：
```bash
./package_mygo_native.sh
```
生成产物：`music-dl-desktop-mygo-native`。

### 自动化测试运行

通过 `ui.NewTester` 在无物理显示环境（Headless）下模拟执行交互、布局与落盘断言：
```bash
go test -v ./desktop_mygo_native/...
```
