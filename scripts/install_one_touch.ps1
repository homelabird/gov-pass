param(
  [switch]$SkipDriverInstall = $false
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$distDir = Join-Path $repoRoot "dist"
$splitterExe = Join-Path $repoRoot "dist\splitter.exe"
$windivertDir = Join-Path $repoRoot "third_party\windivert\WinDivert-2.2.2-A\x64"
$packageScript = Join-Path $repoRoot "scripts\package.ps1"
$installDriverScript = Join-Path $repoRoot "scripts\install_windivert.ps1"

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

$goExe = Resolve-GoCommand

Push-Location -LiteralPath $repoRoot
try {
  New-SafeDirectory $distDir "dist directory"
  Assert-SafeOutputFilePath $splitterExe "splitter.exe"
  & $goExe build -o $splitterExe .\cmd\splitter
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  & $packageScript -ExePath $splitterExe -WinDivertDir $windivertDir -OutDir $distDir
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  if (-not $SkipDriverInstall) {
    & $installDriverScript -WinDivertDir $distDir
    if ($LASTEXITCODE -ne 0) {
      exit $LASTEXITCODE
    }
  }
} finally {
  Pop-Location
}

Write-Host "Installed on Windows: $splitterExe"
if (-not $SkipDriverInstall) {
  Write-Host "WinDivert service install/start complete."
}
