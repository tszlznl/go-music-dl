@echo off
chcp 65001 >nul
echo ========================================================
echo   Go Music DL - Native UI 打包程序 (Pure Go GPU 渲染)
echo ========================================================

go env -w GOPROXY=https://goproxy.cn,direct

echo [1/4] 检查并安装 go-winres 资源编译器...
go install github.com/tc-hib/go-winres@latest

echo [2/4] 生成 Windows 资源清单 (.syso)...
cd winres
go run gen/json.go
cd ..

go-winres make --in winres\desktop_mygo_native.winres.json
if exist rsrc_windows_386.syso move /y rsrc_windows_386.syso desktop_mygo_native\ >nul
if exist rsrc_windows_amd64.syso move /y rsrc_windows_amd64.syso desktop_mygo_native\ >nul
if exist rsrc_windows_arm64.syso move /y rsrc_windows_arm64.syso desktop_mygo_native\ >nul

echo [3/4] 编译纯 Go 原生 GPU 桌面客户端二进制...
cd desktop_mygo_native
go build -ldflags="-H windowsgui -w -s" -o ..\music-dl-desktop-mygo-native.exe .
set BUILD_ERR=%ERRORLEVEL%

if exist *.syso del *.syso >nul
cd ..

if %BUILD_ERR% equ 0 (
    echo ========================================================
    echo [4/4] 编译成功！
    echo 生成的可执行文件: music-dl-desktop-mygo-native.exe
    echo 特点: 纯 Go 编写、无 Webview 依赖、无 CGO 依赖、GPU 原生渲染、内置纯 Go 音频解码播放
    echo ========================================================
) else (
    echo [错误] 编译失败，错误代码: %BUILD_ERR%
)

pause
