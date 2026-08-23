param(
  [switch]$SkipDriverInstall = $false
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$distDir = Join-Path $repoRoot "dist"
$splitterExe = Join-Path $repoRoot "dist\splitter.exe"
$tuiExe = Join-Path $repoRoot "dist\gov-pass-tui.exe"
$windivertDir = Join-Path $repoRoot "third_party\windivert\WinDivert-2.2.2-A\x64"
$packageScript = Join-Path $repoRoot "scripts\package.ps1"
$installDriverScript = Join-Path $repoRoot "scripts\install_windivert.ps1"
$tuiLauncherSource = Join-Path $repoRoot "installer\windows\gov-pass-tui-admin.cmd"
$appServiceName = "gov-pass"

function Test-Admin {
  $id = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($id)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Test-ReparsePoint {
  param([Parameter(Mandatory = $true)][string]$Path)
  $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
  return (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)
}

function Assert-NoReparsePath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $current = [IO.Path]::GetFullPath($Path)
  while (-not [string]::IsNullOrWhiteSpace($current)) {
    if (Test-Path -LiteralPath $current) {
      if (Test-ReparsePoint $current) {
        throw "$Label must not contain a reparse point: $current"
      }
    }
    $parent = Split-Path -Parent $current
    if ([string]::IsNullOrWhiteSpace($parent) -or $parent -eq $current) {
      break
    }
    $current = $parent
  }
}

function Assert-ParentNoReparsePath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Label
  )
  $parent = Split-Path -Parent $Path
  if (-not [string]::IsNullOrWhiteSpace($parent)) {
    Assert-NoReparsePath $parent "$Label parent"
  }
}

function New-SafeDirectory {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Label
  )
  Assert-ParentNoReparsePath $Path $Label
  if (Test-Path -LiteralPath $Path) {
    Assert-NoReparsePath $Path $Label
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
      throw "$Label is not a directory: $Path"
    }
  }
  [void][IO.Directory]::CreateDirectory([IO.Path]::GetFullPath($Path))
  Assert-NoReparsePath $Path $Label
}

function Assert-SafeOutputFilePath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Label
  )
  Assert-ParentNoReparsePath $Path $Label
  if (Test-Path -LiteralPath $Path) {
    if (Test-ReparsePoint $Path) {
      throw "$Label must not be a reparse point: $Path"
    }
    if (Test-Path -LiteralPath $Path -PathType Container) {
      throw "$Label must not be a directory: $Path"
    }
  }
}

function Resolve-ExecutablePath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Label
  )
  if ([string]::IsNullOrWhiteSpace($Path) -or $Path -notmatch '^[A-Za-z]:\\') {
    throw "$Label must be an absolute local path: $Path"
  }
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "$Label not found: $Path"
  }
  if (Test-ReparsePoint $Path) {
    throw "$Label must not be a reparse point: $Path"
  }
  Assert-NoReparsePath $Path $Label
  return (Resolve-Path -LiteralPath $Path).Path
}

function Resolve-GoCommand {
  if (-not [string]::IsNullOrWhiteSpace($env:GOV_PASS_GO_BIN)) {
    return Resolve-ExecutablePath $env:GOV_PASS_GO_BIN "GOV_PASS_GO_BIN"
  }

  $programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
  if (-not [string]::IsNullOrWhiteSpace($programFiles)) {
    $goPath = Join-Path $programFiles "Go\bin\go.exe"
    if (Test-Path -LiteralPath $goPath -PathType Leaf) {
      return Resolve-ExecutablePath $goPath "go.exe"
    }
  }

  throw "go.exe not found in the standard Go install directory. Set GOV_PASS_GO_BIN to an absolute go.exe path."
}

function Get-System32Command {
  param([Parameter(Mandatory = $true)][string]$Name)
  if ([IO.Path]::GetFileName($Name) -ne $Name) {
    throw "System32 command name must be a bare filename: $Name"
  }
  $systemDirectory = [Environment]::SystemDirectory
  if ([string]::IsNullOrWhiteSpace($systemDirectory)) {
    throw "System32 directory could not be resolved."
  }
  $path = Join-Path $systemDirectory $Name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "Required System32 command not found: $path"
  }
  Assert-NoReparsePath $path "System32 command"
  return $path
}

if (-not $SkipDriverInstall -and -not (Test-Admin)) {
  Write-Error "Administrator privileges required."
  exit 1
}

if (-not (Test-Path -LiteralPath $packageScript -PathType Leaf)) {
  Write-Error "package.ps1 not found: $packageScript"
  exit 1
}
if (Test-ReparsePoint $packageScript) {
  Write-Error "package.ps1 must not be a reparse point: $packageScript"
  exit 1
}
Assert-NoReparsePath $packageScript "package.ps1"

if (-not (Test-Path -LiteralPath $installDriverScript -PathType Leaf)) {
  Write-Error "install_windivert.ps1 not found: $installDriverScript"
  exit 1
}
if (Test-ReparsePoint $installDriverScript) {
  Write-Error "install_windivert.ps1 must not be a reparse point: $installDriverScript"
  exit 1
}
Assert-NoReparsePath $installDriverScript "install_windivert.ps1"

if (-not (Test-Path -LiteralPath $tuiLauncherSource -PathType Leaf)) {
  Write-Error "gov-pass-tui-admin.cmd not found: $tuiLauncherSource"
  exit 1
}
Assert-NoReparsePath $tuiLauncherSource "gov-pass-tui-admin.cmd"

$goExe = Resolve-GoCommand

Push-Location -LiteralPath $repoRoot
try {
  New-SafeDirectory $distDir "dist directory"
  Assert-SafeOutputFilePath $splitterExe "splitter.exe"
  & $goExe build -o $splitterExe .\cmd\splitter
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
  Assert-SafeOutputFilePath $tuiExe "gov-pass-tui.exe"
  & $goExe build -o $tuiExe .\cmd\gov-pass-tui
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  & $packageScript -ExePath $splitterExe -WinDivertDir $windivertDir -OutDir $distDir
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  if (-not $SkipDriverInstall) {
    $programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
    if ([string]::IsNullOrWhiteSpace($programFiles)) {
      throw "Program Files directory could not be resolved."
    }
    $installRoot = Join-Path $programFiles "gov-pass"
    $installedSplitter = Join-Path $installRoot "splitter.exe"
    $installedTui = Join-Path $installRoot "gov-pass-tui.exe"
    $installedTuiLauncher = Join-Path $installRoot "gov-pass-tui-admin.cmd"

    $existingService = Get-Service -Name $appServiceName -ErrorAction SilentlyContinue
    if ($existingService -and $existingService.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
      Stop-Service -Name $appServiceName -ErrorAction Stop
      $existingService.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(30))
    }

    New-SafeDirectory $installRoot "install directory"
    & $packageScript -ExePath $splitterExe -WinDivertDir $windivertDir -OutDir $installRoot
    if ($LASTEXITCODE -ne 0) {
      exit $LASTEXITCODE
    }

    Assert-SafeOutputFilePath $installedTui "installed gov-pass-tui.exe"
    [IO.File]::Copy($tuiExe, $installedTui, $true)
    Assert-SafeOutputFilePath $installedTuiLauncher "installed gov-pass-tui-admin.cmd"
    [IO.File]::Copy($tuiLauncherSource, $installedTuiLauncher, $true)

    & $installDriverScript -WinDivertDir $installRoot
    if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne 1056) {
      exit $LASTEXITCODE
    }

    $scExe = Get-System32Command "sc.exe"
    $serviceBinaryPath = "`"$installedSplitter`" --service --service-name $appServiceName"
    if ($existingService) {
      $serviceOutput = & $scExe config $appServiceName start= auto binPath= $serviceBinaryPath 2>&1
    } else {
      $serviceOutput = & $scExe create $appServiceName type= own start= auto error= normal binPath= $serviceBinaryPath DisplayName= "gov-pass splitter" 2>&1
    }
    $serviceExit = $LASTEXITCODE
    $serviceOutput | Out-Host
    if ($serviceExit -ne 0) {
      throw "Failed to create or update the gov-pass service (exit $serviceExit)."
    }
    & $scExe description $appServiceName "Split-only TLS ClientHello splitter (WinDivert)" | Out-Host
    if ($LASTEXITCODE -ne 0) {
      throw "Failed to set the gov-pass service description."
    }

    Start-Service -Name $appServiceName -ErrorAction Stop
    $service = Get-Service -Name $appServiceName -ErrorAction Stop
    $service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(30))

    $commonAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
    if ([string]::IsNullOrWhiteSpace($commonAppData)) {
      throw "ProgramData directory could not be resolved."
    }
    $configPath = Join-Path (Join-Path $commonAppData "gov-pass") "config.json"
    $configDeadline = [DateTime]::UtcNow.AddSeconds(30)
    while (-not (Test-Path -LiteralPath $configPath -PathType Leaf) -and [DateTime]::UtcNow -lt $configDeadline) {
      Start-Sleep -Milliseconds 250
      $service.Refresh()
      if ($service.Status -eq [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
        throw "gov-pass service stopped during startup."
      }
    }
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
      throw "gov-pass service did not create its default config: $configPath"
    }
    Assert-NoReparsePath $configPath "default config"
    $service.Refresh()
    if ($service.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Running) {
      throw "gov-pass service is not running after startup (status: $($service.Status))."
    }

    $programs = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonPrograms)
    if ([string]::IsNullOrWhiteSpace($programs)) {
      throw "Common Start menu directory could not be resolved."
    }
    $menuDir = Join-Path $programs "gov-pass"
    New-SafeDirectory $menuDir "Start menu directory"
    $shortcutPath = Join-Path $menuDir "gov-pass TUI.lnk"
    Assert-SafeOutputFilePath $shortcutPath "Start menu shortcut"
    $shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($shortcutPath)
    $shortcut.TargetPath = $installedTuiLauncher
    $shortcut.WorkingDirectory = $installRoot
    $shortcut.Description = "Manage the gov-pass service as Administrator"
    $shortcut.Save()
  }
} finally {
  Pop-Location
}

if ($SkipDriverInstall) {
  Write-Host "Built Windows files in: $distDir"
  Write-Host "Driver and service installation skipped."
} else {
  Write-Host "Installed on Windows: $installRoot"
  Write-Host "gov-pass service and WinDivert are running."
  Write-Host "Launch 'gov-pass TUI' from the Start menu."
}
