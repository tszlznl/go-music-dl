# Go Music DL - Native UI (Pure Go GPU 渲染原生桌面端)

基于 [egoist/mygo](https://github.com/egoist/mygo) 原生 UI 引擎（`github.com/egoist/mygo/ui`）构建的跨平台音乐搜索、试听与下载桌面客户端。

## 🌟 核心特色

1. **纯 Go 编写，零 CGO 依赖**：
   - 跨平台编译无需安装 gcc、clang 等 C 语言工具链，即开即用。
2. **纯 GPU 原生声明式渲染**：
   - 无 Webview、无 Chromium、无 HTML/CSS/JS，内存占用极低（通常只需 20~40MB），启动速度毫秒级。
3. **内置纯 Go 音频解码播放**：
   - 基于 `github.com/egoist/mygo/audio` 与 `github.com/egoist/mygo/audio/mp3`，实现纯 Go MP3 音频流式解码与系统扬声器输出，支持音量调节、暂停与恢复。
4. **全网多平台音乐聚合搜索**：
   - 支持网易云音乐、QQ音乐、酷狗音乐、酷我音乐、咪咕音乐、Bilibili、汽水音乐等主流音源并发搜索。
5. **完整下载管理与系统交互**：
   - 支持异步下载、无损音质获取、封面歌词嵌入、保存路径自定义选择（调用系统原生目录对话框 `mygo.Dialog.Open`）与在文件管理器中定位（`mygo.Shell.ShowItemInFolder`）。
   - 支持系统桌面通知（`mygo.NewNotification`）。
   - 单实例运行保护（`mygo.App.RequestSingleInstanceLock`）。

## 📁 目录结构

```
desktop_mygo_native/
├── main.go             # 原生 UI 客户端主体逻辑（侧边栏、搜索页、下载页、设置页、底部播放条）
├── main_test.go        # 基于 ui.Tester 的自动化测试套件
├── icon.png            # 嵌入式应用图标资源
└── app.desktop         # Linux 桌面快捷方式配置
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

Windows（生成带图标且无终端黑框的 Windows GUI 可执行文件）：
```bat
package_mygo_native.bat
```

Linux / macOS：
```bash
./package_mygo_native.sh
```

### 单元测试运行

通过 `ui.NewTester` 在无物理显示环境（Headless）下模拟执行交互与布局断言：
```bash
go test -v ./desktop_mygo_native/...
```
