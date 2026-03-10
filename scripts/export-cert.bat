@echo off
REM 导出 DeskGo 自签名证书

echo ========================================
echo 导出 DeskGo 开发证书
echo ========================================
echo.

set CERT_NAME=DeskGo-Dev-Certificate

REM 检查证书是否存在
powershell -Command "Get-ChildItem Cert:\LocalMachine\My | Where-Object {$_.Subject -like '*%CERT_NAME%*'}" >nul 2>&1

if %errorLevel% neq 0 (
    echo [错误] 未找到证书！
    echo 请先运行 sign-windows.bat 创建证书
    pause
    exit /b 1
)

REM 获取证书指纹
for /f "tokens=*" %%i in ('powershell -Command "(Get-ChildItem Cert:\LocalMachine\My | Where-Object {$_.Subject -like '*%CERT_NAME%*'}).Thumbprint"') do set CERT_THUMBPRINT=%%i

echo 证书指纹: %CERT_THUMBPRINT%
echo.

REM 导出证书到当前目录
echo 正在导出证书...
powershell -Command "Export-Certificate -Cert Cert:\LocalMachine\My\%CERT_THUMBPRINT% -FilePath DeskGo-Dev.cer -Type CERT"

if %errorLevel% equ 0 (
    echo.
    echo ========================================
    echo ✓ 证书导出成功！
    echo.
    echo 输出文件：DeskGo-Dev.cer
    echo.
    echo 用户安装方法：
    echo 1. 双击 DeskGo-Dev.cer
    echo 2. 点击"安装证书"
    echo 3. 选择"本地计算机"
    echo 4. 选择"将所有证书放入下列存储"
    echo 5. 浏览到"受信任的根证书颁发机构"
    echo 6. 点击"完成"
    echo ========================================
) else (
    echo [错误] 证书导出失败
)

echo.
pause
