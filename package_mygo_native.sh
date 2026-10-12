#!/usr/bin/env bash
set -e

echo "========================================================"
echo "  Go Music DL - Native UI 打包程序 (Linux / macOS)"
echo "========================================================"

export GOPROXY="https://goproxy.cn,direct"

OUTPUT_NAME="music-dl-desktop-mygo-native"
if [ "$(uname)" = "Darwin" ]; then
    echo "检测到运行环境: macOS"
else
    echo "检测到运行环境: Linux"
fi

echo "[1/2] 整理依赖项并运行单元测试..."
go test -v ./desktop_mygo_native/...

echo "[2/2] 编译纯 Go 原生 UI 桌面可执行文件..."
go build -ldflags="-w -s" -o "${OUTPUT_NAME}" ./desktop_mygo_native

echo "========================================================"
echo "编译成功！生成的可执行文件: ${OUTPUT_NAME}"
echo "特点: 纯 Go 编写、无 Webview 依赖、无 CGO 依赖、GPU 原生渲染、内置纯 Go 音频解码播放"
echo "========================================================"
