@echo off
chcp 65001 >nul
echo 启动 Go Music DL Native UI 桌面端...
cd desktop_mygo_native
go run . %*
cd ..
