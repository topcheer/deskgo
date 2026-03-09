[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'uninstall')]
    [string]$Action,

    [Alias('relay-server')]
    [string]$RelayServer,
    [ValidateSet('jpeg', 'h264')]
    [string]$Codec,
    [string]$Session,
    [string]$Version = 'latest',
    [string]$Repository = 'topcheer/deskgo',
    [Alias('autostart-mode')]
    [ValidateSet('scheduled-task')]
    [string]$AutostartMode,
    [Alias('non-interactive')]
    [switch]$NonInteractive,
    [Alias('dry-run')]
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$TaskName = 'DeskGo Desktop CLI'

function Write-Info {
    param([string]$Message)
    Write-Host "ℹ️  $Message"
}

function Write-Success {
    param([string]$Message)
    Write-Host "✅ $Message"
}

function Write-WarnMessage {
    param([string]$Message)
    Write-Warning $Message
}

function Fail {
    param([string]$Message)
    throw $Message
}

function Read-DefaultValue {
    param(
        [string]$Prompt,
        [string]$DefaultValue
    )

    $answer = Read-Host "$Prompt [$DefaultValue]"
    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $DefaultValue
    }
    return $answer.Trim()
}

function Confirm-Continue {
    param([string]$Prompt)

    while ($true) {
        $answer = Read-Host "$Prompt [Y/n]"
        if ([string]::IsNullOrWhiteSpace($answer) -or $answer -match '^(?i:y|yes)$') {
            return $true
        }
        if ($answer -match '^(?i:n|no)$') {
            return $false
        }
        Write-WarnMessage '请输入 y 或 n。'
    }
}

function Normalize-Version {
    param([string]$Value)

    if ($Value -eq 'latest') {
        return $Value
    }
    if ($Value.StartsWith('v')) {
        return $Value
    }
    return "v$Value"
}

function Normalize-SessionName {
    param([string]$Value)

    $normalized = ($Value -replace '[^A-Za-z0-9._-]+', '-').Trim('-')
    $normalized = $normalized -replace '-{2,}', '-'
    return $normalized.ToLowerInvariant()
}

function Get-DefaultSessionName {
    $candidate = Normalize-SessionName -Value $env:COMPUTERNAME
    if ([string]::IsNullOrWhiteSpace($candidate)) {
        return 'deskgo-host'
    }
    return $candidate
}

function Normalize-RelayServerUrl {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        Fail 'Relay 地址不能为空。'
    }

    $trimmed = $Value.Trim().TrimEnd('/')
    if ($trimmed -notmatch '^[a-zA-Z][a-zA-Z0-9+.-]*://') {
        return "wss://$trimmed/api/desktop"
    }

    $uri = [Uri]$trimmed
    $builder = [UriBuilder]$uri
    switch ($uri.Scheme.ToLowerInvariant()) {
        'http' {
            $builder.Scheme = 'ws'
        }
        'https' {
            $builder.Scheme = 'wss'
        }
        'ws' {
            $builder.Scheme = 'ws'
        }
        'wss' {
            $builder.Scheme = 'wss'
        }
        default {
            Fail "不支持的 Relay URL scheme: $($uri.Scheme)"
        }
    }

    $path = $builder.Path.TrimEnd('/')
    if ([string]::IsNullOrWhiteSpace($path)) {
        $builder.Path = '/api/desktop'
    }
    elseif (-not $path.EndsWith('/api/desktop')) {
        $builder.Path = "$path/api/desktop"
    }

    return $builder.Uri.AbsoluteUri.TrimEnd('/')
}

function Resolve-ReleaseTag {
    param([string]$RequestedVersion)

    $normalized = Normalize-Version -Value $RequestedVersion
    if ($normalized -ne 'latest') {
        return $normalized
    }

    Write-Info "正在解析 $Repository 的 latest release..."
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest" -Headers @{ 'Accept' = 'application/vnd.github+json' }
    if (-not $release.tag_name) {
        Fail '无法解析 latest release 标签。'
    }
    return [string]$release.tag_name
}

function Get-CurrentArchitecture {
    $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    switch ($architecture) {
        'X64' { return 'amd64' }
        'Arm64' { return 'arm64' }
        default { Fail "不支持的 Windows 架构: $architecture" }
    }
}

function Test-IsWindowsHost {
    return [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT
}

function Get-InstallRoot {
    if (Test-IsWindowsHost) {
        if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            return (Join-Path $env:LOCALAPPDATA 'DeskGo')
        }
        if (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
            return (Join-Path $env:APPDATA 'DeskGo')
        }
        Fail '无法确定 Windows 用户安装目录。'
    }

    if ($DryRun) {
        $previewRoot = Join-Path $HOME '.deskgo-windows-preview'
        Write-Info "检测到非 Windows PowerShell 主机，使用 dry-run 预览路径: $previewRoot"
        return $previewRoot
    }

    Fail '该 PowerShell 自动运行脚本仅支持 Windows。'
}

function Ensure-CodecSupported {
    param([string]$CodecValue)

    if ($CodecValue -ne 'jpeg') {
        Fail '当前 Windows 自动运行模式仅支持 JPEG。H.264 目前没有 Windows 自动运行编码实现。'
    }
}

function Write-FileContent {
    param(
        [string]$Path,
        [string]$Content
    )

    if ($DryRun) {
        Write-Info "将写入文件: $Path"
        return
    }

    $directory = Split-Path -Parent $Path
    if ($directory) {
        New-Item -ItemType Directory -Force -Path $directory | Out-Null
    }
    $extension = [System.IO.Path]::GetExtension($Path)
    $encoding = if ($extension -ieq '.ps1') {
        New-Object System.Text.UTF8Encoding($true)
    }
    else {
        New-Object System.Text.UTF8Encoding($false)
    }
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Remove-PathSafe {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    if ($DryRun) {
        Write-Info "将删除: $Path"
        return
    }

    Remove-Item -LiteralPath $Path -Recurse -Force
}

function Download-File {
    param(
        [string]$Url,
        [string]$Destination
    )

    if ($DryRun) {
        Write-Info "将下载: $Url -> $Destination"
        return
    }

    $directory = Split-Path -Parent $Destination
    if ($directory) {
        New-Item -ItemType Directory -Force -Path $directory | Out-Null
    }
    Invoke-WebRequest -Uri $Url -OutFile $Destination
}

function Get-ChecksumValue {
    param(
        [string]$ChecksumFile,
        [string]$AssetName
    )

    foreach ($line in Get-Content -LiteralPath $ChecksumFile) {
        if ($line -match '^(?<hash>[A-Fa-f0-9]+)\s+\*?(?<name>.+)$' -and $Matches.name -eq $AssetName) {
            return $Matches.hash.ToLowerInvariant()
        }
    }

    Fail "SHA256SUMS.txt 中未找到 $AssetName"
}

function New-LauncherContent {
    param(
        [string]$BinaryPath,
        [string]$ConfigPath,
        [string]$LogDir
    )

    $escapedBinary = $BinaryPath.Replace("'", "''")
    $escapedConfig = $ConfigPath.Replace("'", "''")
    $escapedLogDir = $LogDir.Replace("'", "''")

    return @"
`$ErrorActionPreference = 'Stop'
`$binary = '$escapedBinary'
`$configPath = '$escapedConfig'
`$logDir = '$escapedLogDir'
`$installRoot = Split-Path -Parent `$PSCommandPath
`$logPath = Join-Path `$logDir 'desktop.log'
`$startupDelaySeconds = 15
`$maxAttempts = 4

function Write-LauncherLog {
    param([string]`$Message)
    `$timestamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    "[`$timestamp] `$Message" | Out-File -FilePath `$logPath -Append -Encoding utf8
}

try {
    New-Item -ItemType Directory -Force -Path `$logDir | Out-Null
    Set-Location -LiteralPath `$installRoot
    Write-LauncherLog "DeskGo launcher entered. Working directory: `$installRoot"
    if (-not (Test-Path -LiteralPath `$configPath)) {
        throw "Config file missing: `$configPath"
    }
    if (-not (Test-Path -LiteralPath `$binary)) {
        throw "Binary file missing: `$binary"
    }
    if (`$startupDelaySeconds -gt 0) {
        Write-LauncherLog "Delaying startup for `$startupDelaySeconds seconds after logon."
        Start-Sleep -Seconds `$startupDelaySeconds
    }

    for (`$attempt = 1; `$attempt -le `$maxAttempts; `$attempt++) {
        Write-LauncherLog "Starting DeskGo Desktop CLI (attempt=`$attempt/`$maxAttempts)."
        & `$binary *>> `$logPath
        `$exitCode = `$LASTEXITCODE
        if (`$exitCode -eq 0) {
            Write-LauncherLog "DeskGo Desktop CLI exited normally."
            exit 0
        }

        Write-LauncherLog "DeskGo Desktop CLI exited with code `$exitCode."
        if (`$attempt -ge `$maxAttempts) {
            exit `$exitCode
        }

        `$retryDelaySeconds = 10 * `$attempt
        Write-LauncherLog "Retrying in `$retryDelaySeconds seconds."
        Start-Sleep -Seconds `$retryDelaySeconds
    }
}
catch {
    Write-LauncherLog ('Launcher error: ' + `$_.Exception.Message)
    exit 1
}
"@
}

function Register-DeskGoTask {
    param(
        [string]$LauncherPath,
        [string]$TaskName
    )

    if ($DryRun) {
        Write-Info "将注册计划任务: $TaskName"
        return
    }

    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    $taskAction = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument ("-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File `"{0}`"" -f $LauncherPath)
    $taskTrigger = New-ScheduledTaskTrigger -AtLogOn -User $currentUser
    $taskPrincipal = New-ScheduledTaskPrincipal -UserId $currentUser -LogonType Interactive -RunLevel Limited
    $taskSettings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -MultipleInstances IgnoreNew -StartWhenAvailable

    Register-ScheduledTask -TaskName $TaskName -Action $taskAction -Trigger $taskTrigger -Principal $taskPrincipal -Settings $taskSettings -Force | Out-Null
    Start-ScheduledTask -TaskName $TaskName
}

function Unregister-DeskGoTask {
    param([string]$TaskName)

    if ($DryRun) {
        Write-Info "将注销计划任务: $TaskName"
        return
    }

    $existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
    }
}

function Stop-DeskGoTask {
    param([string]$TaskName)

    if ($DryRun) {
        Write-Info "将停止计划任务（如果正在运行）: $TaskName"
        return
    }

    $existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($existing -and $existing.State -eq 'Running') {
        Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 1
    }
}

function Stop-DeskGoProcess {
    param([string]$BinaryPath)

    if ($DryRun) {
        Write-Info "将停止旧的 DeskGo 进程（如果存在）: $BinaryPath"
        return
    }

    $targetPath = [System.IO.Path]::GetFullPath($BinaryPath)
    $processes = Get-Process -Name 'deskgo-desktop' -ErrorAction SilentlyContinue | Where-Object {
        try {
            $_.Path -and ([System.IO.Path]::GetFullPath($_.Path) -eq $targetPath)
        }
        catch {
            $false
        }
    }

    foreach ($process in $processes) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}

function Show-InstallSummary {
    param(
        [string]$Architecture,
        [string]$ResolvedVersion,
        [string]$ResolvedRelay,
        [string]$ResolvedSession,
        [string]$ResolvedCodec,
        [string]$InstallRoot,
        [string]$BinaryPath,
        [string]$ConfigPath,
        [string]$LauncherPath,
        [string]$LogDir
    )

    Write-Host ""
    Write-Host 'DeskGo 自动运行配置摘要'
    Write-Host "  平台/架构 : windows/$Architecture"
    Write-Host "  下载仓库  : $Repository"
    Write-Host "  Release    : $ResolvedVersion"
    Write-Host "  Relay      : $ResolvedRelay"
    Write-Host "  Codec      : $ResolvedCodec"
    Write-Host "  Session    : $ResolvedSession"
    Write-Host '  模式       : scheduled-task'
    Write-Host "  安装目录   : $InstallRoot"
    Write-Host "  二进制路径 : $BinaryPath"
    Write-Host "  配置文件   : $ConfigPath（通过工作目录兼容旧 release）"
    Write-Host "  启动脚本   : $LauncherPath"
    Write-Host "  日志目录   : $LogDir"
    Write-Host "  计划任务   : $TaskName"
    Write-Host '  登录行为   : 隐藏启动，登录后会延迟约 15 秒再尝试连接 Relay'
    Write-Host ""
}

function Prompt-ActionIfNeeded {
    if ($Action) {
        return
    }

    if ($NonInteractive) {
        Fail '非引导模式必须显式指定 install 或 uninstall。'
    }

    while (-not $Action) {
        $choice = (Read-DefaultValue -Prompt '请选择操作（install / uninstall）' -DefaultValue 'install').ToLowerInvariant()
        switch ($choice) {
            'install' { $script:Action = 'install' }
            'uninstall' { $script:Action = 'uninstall' }
            default { Write-WarnMessage '请输入 install 或 uninstall。' }
        }
    }
}

function Collect-InteractiveInstallOptions {
    Write-Info '进入 DeskGo 自动运行引导安装。'

    if (-not $RelayServer) {
        $script:RelayServer = Read-DefaultValue -Prompt 'Relay 地址（支持 https://host 或 wss://host/api/desktop）' -DefaultValue 'wss://deskgo.zty8.cn/api/desktop'
    }

    if (-not $Codec) {
        $script:Codec = Read-DefaultValue -Prompt 'Codec（Windows 当前仅支持 jpeg）' -DefaultValue 'jpeg'
    }

    if (-not $Session) {
        $script:Session = Read-DefaultValue -Prompt '固定 Session 名字' -DefaultValue (Get-DefaultSessionName)
    }

    if (-not $Version) {
        $script:Version = 'latest'
    }
    $script:Version = Read-DefaultValue -Prompt '安装的 release 版本' -DefaultValue $Version
}

function Install-DeskGoAutostart {
    $architecture = Get-CurrentArchitecture
    $resolvedVersion = Normalize-Version -Value $Version
    $relayInput = if ([string]::IsNullOrWhiteSpace($RelayServer)) { 'wss://deskgo.zty8.cn/api/desktop' } else { $RelayServer }
    $resolvedRelay = Normalize-RelayServerUrl -Value $relayInput
    $resolvedCodec = if ([string]::IsNullOrWhiteSpace($Codec)) { 'jpeg' } else { $Codec.ToLowerInvariant() }
    Ensure-CodecSupported -CodecValue $resolvedCodec

    $sessionInput = if ([string]::IsNullOrWhiteSpace($Session)) { Get-DefaultSessionName } else { $Session }
    $resolvedSession = Normalize-SessionName -Value $sessionInput
    if ([string]::IsNullOrWhiteSpace($resolvedSession)) {
        Fail 'Session 名字为空，请使用字母、数字、点、下划线或连字符。'
    }

    if ($AutostartMode -and $AutostartMode -ne 'scheduled-task') {
        Fail 'Windows 仅支持 scheduled-task 自动运行模式。'
    }

    $installRoot = Get-InstallRoot
    $binaryPath = Join-Path $installRoot 'bin\deskgo-desktop.exe'
    $configPath = Join-Path $installRoot 'deskgo.json'
    $launcherPath = Join-Path $installRoot 'run-desktop.ps1'
    $logDir = Join-Path $installRoot 'logs'
    $artifactName = "deskgo-desktop-windows-$architecture.exe"

    Show-InstallSummary -Architecture $architecture -ResolvedVersion $resolvedVersion -ResolvedRelay $resolvedRelay -ResolvedSession $resolvedSession -ResolvedCodec $resolvedCodec -InstallRoot $installRoot -BinaryPath $binaryPath -ConfigPath $configPath -LauncherPath $launcherPath -LogDir $logDir

    if (-not $NonInteractive -and -not (Confirm-Continue -Prompt '确认继续安装吗？')) {
        Fail '已取消安装。'
    }

    $resolvedVersion = Resolve-ReleaseTag -RequestedVersion $resolvedVersion
    $assetUrl = "https://github.com/$Repository/releases/download/$resolvedVersion/$artifactName"
    $checksumUrl = "https://github.com/$Repository/releases/download/$resolvedVersion/SHA256SUMS.txt"
    Write-Info "将安装版本: $resolvedVersion"
    Write-Info "下载产物: $artifactName"
    Write-Info "下载地址: $assetUrl"

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
    $tempBinary = Join-Path $tempDir $artifactName
    $tempChecksum = Join-Path $tempDir 'SHA256SUMS.txt'

    if (-not $DryRun) {
        New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
    }

    Download-File -Url $assetUrl -Destination $tempBinary
    Download-File -Url $checksumUrl -Destination $tempChecksum

    Stop-DeskGoTask -TaskName $TaskName
    Stop-DeskGoProcess -BinaryPath $binaryPath

    if (-not $DryRun) {
        $expected = Get-ChecksumValue -ChecksumFile $tempChecksum -AssetName $artifactName
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $tempBinary).Hash.ToLowerInvariant()
        if ($expected -ne $actual) {
            Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
            Fail "SHA256 校验失败。expected=$expected actual=$actual"
        }

        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $binaryPath) | Out-Null
        Move-Item -Force -Path $tempBinary -Destination $binaryPath
    }

    $configObject = [ordered]@{
        server  = $resolvedRelay
        session = $resolvedSession
        codec   = $resolvedCodec
    }
    $configContent = $configObject | ConvertTo-Json -Depth 4
    Write-FileContent -Path $configPath -Content $configContent
    Write-FileContent -Path $launcherPath -Content (New-LauncherContent -BinaryPath $binaryPath -ConfigPath $configPath -LogDir $logDir)

    Register-DeskGoTask -LauncherPath $launcherPath -TaskName $TaskName

    if (-not $DryRun -and (Test-Path -LiteralPath $tempDir)) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    Write-Success 'DeskGo 自动运行安装完成。'
}

function Uninstall-DeskGoAutostart {
    $installRoot = Get-InstallRoot
    $binaryPath = Join-Path $installRoot 'bin\deskgo-desktop.exe'

    Write-Host ''
    Write-Host 'DeskGo 自动运行卸载摘要'
    Write-Host "  安装目录 : $installRoot"
    Write-Host "  计划任务 : $TaskName"
    Write-Host ''

    if (-not $NonInteractive -and -not (Confirm-Continue -Prompt '确认卸载当前用户下的 DeskGo 自动运行配置吗？')) {
        Fail '已取消卸载。'
    }

    Stop-DeskGoTask -TaskName $TaskName
    Stop-DeskGoProcess -BinaryPath $binaryPath
    Unregister-DeskGoTask -TaskName $TaskName
    Remove-PathSafe -Path $installRoot

    Write-Success 'DeskGo 自动运行已卸载。'
}

Prompt-ActionIfNeeded

switch ($Action) {
    'install' {
        if (-not $NonInteractive) {
            Collect-InteractiveInstallOptions
        }
        Install-DeskGoAutostart
    }
    'uninstall' {
        Uninstall-DeskGoAutostart
    }
    default {
        Fail "未知操作: $Action"
    }
}
