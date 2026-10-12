#!/usr/bin/env bash
set -e

echo "启动 Go Music DL Native UI 桌面端..."
exec go run ./desktop_mygo_native "$@"
