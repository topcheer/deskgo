@echo off
REM DeskGo Windows 版本编译脚本

echo ========================================
echo DeskGo Windows 编译脚本
echo ========================================
echo.

REM 检查 Go 是否安装
where go >nul 2>&1
if %errorLevel% neq 0 (
    echo [错误] 未找到 Go！请先安装 Go: https://golang.org/dl/
    pause
    exit /b 1
)

echo ✓ 找到 Go 编译器
go version
echo.

REM 创建 bin 目录
if not exist bin mkdir bin

echo [步骤 1] 编译 Relay 服务器...
go build -o bin\relay-server.exe .\cmd\relay
if %errorLevel% equ 0 (
    echo ✓ Relay 服务器编译成功
) else (
    echo ✗ Relay 服务器编译失败
    pause
    exit /b 1
)
echo.

echo [步骤 2] 编译桌面捕获客户端 (带 H.264 支持)...
go build -tags desktop -o bin\deskgo-desktop-h264.exe .\cmd\client
if %errorLevel% equ 0 (
    echo ✓ 桌面捕获客户端编译成功
) else (
    echo ✗ 桌面捕获客户端编译失败
    pause
    exit /b 1
)
echo.

echo ========================================
echo 编译完成！
echo 输出文件：
echo   - bin\relay-server.exe
echo   - bin\deskgo-desktop-h264.exe
echo ========================================
echo.
echo 下一步：运行 sign-windows.bat 进行签名
echo.
pause
