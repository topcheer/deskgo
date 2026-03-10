@echo off
REM DeskGo Windows 可执行文件签名脚本
REM 用法：sign-windows.bat

echo ========================================
echo DeskGo Windows Exe 签名脚本
echo ========================================
echo.

REM 设置证书名称（会创建/使用这个证书）
set CERT_NAME=DeskGo-Dev-Certificate
set CERT_SUBJECT=CN=DeskGo Development, O=DeskGo, C=CN

REM 检查是否以管理员权限运行
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [错误] 请以管理员权限运行此脚本！
    echo 右键点击脚本，选择"以管理员身份运行"
    pause
    exit /b 1
)

echo [步骤 1] 检查是否存在自签名证书...
powershell -Command "Get-ChildItem Cert:\LocalMachine\My | Where-Object {$_.Subject -like '*%CERT_NAME%*'}" >nul 2>&1

if %errorLevel% neq 0 (
    echo [证书不存在] 正在创建新的自签名证书...
    powershell -Command "$cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject '%CERT_SUBJECT%' -FriendlyName '%CERT_NAME%' -CertStoreLocation 'Cert:\LocalMachine\My' -KeyUsage DigitalSignature -KeyLength 2048 -NotAfter (Get-Date).AddYears(10); Write-Host '✓ 证书创建成功，指纹: ' $cert.Thumbprint"

    if %errorLevel% neq 0 (
        echo [错误] 证书创建失败！
        pause
        exit /b 1
    )
) else (
    echo ✓ 找到现有证书
)

echo.
echo [步骤 2] 查找 SignTool.exe...

REM 尝试查找 signtool.exe（可能在 Visual Studio 或 Windows SDK 中）
set SIGNTOOL=

REM 检查常见的 Visual Studio 路径
for /f "delims=" %%i in ('dir /b /s "C:\Program Files (x86)\Windows Kits\10\bin\*signtool.exe" 2^>nul ^| findstr /i "x64"') do (
    set SIGNTOOL=%%i
    goto :found_signtool
)

for /f "delims=" %%i in ('dir /b /s "C:\Program Files\Windows Kits\10\bin\*signtool.exe" 2^>nul ^| findstr /i "x64"') do (
    set SIGNTOOL=%%i
    goto :found_signtool
)

echo [错误] 未找到 SignTool.exe！
echo 请安装 Windows SDK 或 Visual Studio
pause
exit /b 1

:found_signtool
echo ✓ 找到 SignTool: %SIGNTOOL%
echo.

REM 获取证书指纹
for /f "tokens=*" %%i in ('powershell -Command "(Get-ChildItem Cert:\LocalMachine\My | Where-Object {$_.Subject -like '*%CERT_NAME%*'}).Thumbprint"') do set CERT_THUMBPRINT=%%i

echo [步骤 3] 签名可执行文件...
echo.

REM 签名 bin 目录下的所有 .exe 和 .dll 文件
if exist "bin\*.exe" (
    for %%f in (bin\*.exe) do (
        echo 正在签名: %%f
        "%SIGNTOOL%" sign /f "Cert:\LocalMachine\My\%CERT_THUMBPRINT%" /fd sha256 /tr http://timestamp.digicert.com /td sha256 "%%f"

        if %errorLevel% equ 0 (
            echo ✓ 签名成功: %%f
        ) else (
            echo ✗ 签名失败: %%f
        )
        echo.
    )
) else (
    echo [提示] bin 目录中没有找到 .exe 文件
    echo 请先运行 build.bat 编译 Windows 版本
)

echo [步骤 4] 验证签名...
if exist "bin\*.exe" (
    for %%f in (bin\*.exe) do (
        echo.
        echo 验证: %%f
        "%SIGNTOOL%" verify /pa "%%f"
    )
)

echo.
echo ========================================
echo 签名完成！
echo.
echo 重要提示：
echo 1. 这是自签名证书，首次运行会显示安全警告
echo 2. 用户需要手动信任此证书
echo 3. 建议将证书导出供用户安装
echo.
echo 导出证书命令：
echo powershell -Command "Export-Certificate -Cert Cert:\LocalMachine\My\%CERT_THUMBPRINT% -FilePath DeskGo-Dev.cer"
echo ========================================
pause
