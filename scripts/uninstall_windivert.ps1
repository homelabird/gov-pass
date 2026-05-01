param(
  [string]$ServiceName = "WinDivert"
)

$ErrorActionPreference = "Stop"

function Test-Admin {
  $id = [Security.Principal.WindowsIdentity]::GetCurrent()
  $p = New-Object Security.Principal.WindowsPrincipal($id)
  return $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Test-ReparsePoint([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) {
    return $false
  }
  $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
  return (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)
}

function Assert-NoReparsePath([string]$Path, [string]$Label) {
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

function Get-System32Command([string]$Name) {
  if ([IO.Path]::GetFileName($Name) -ne $Name) {
    throw "System32 command name must be a bare filename: $Name"
  }
  $sysDir = [Environment]::SystemDirectory
  if ([string]::IsNullOrWhiteSpace($sysDir)) {
    throw "System32 directory could not be resolved."
  }
  $path = Join-Path $sysDir $Name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "Required System32 command not found: $path"
  }
  if (Test-ReparsePoint $path) {
    throw "Required System32 command must not be a reparse point: $path"
  }
  Assert-NoReparsePath $path "Required System32 command"
  return $path
}

function Assert-ServiceName([string]$Name) {
  if ([string]::IsNullOrWhiteSpace($Name)) {
    throw "ServiceName is empty."
  }
  if ($Name -notmatch '^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$') {
    throw "ServiceName is invalid; use A-Z, a-z, 0-9, '-' or '_'."
  }
}

if (-not (Test-Admin)) {
  Write-Error "Administrator privileges required."
  exit 1
}

try {
  Assert-ServiceName $ServiceName
  $ScExe = Get-System32Command "sc.exe"
} catch {
  Write-Error $_.Exception.Message
  exit 1
}

& $ScExe stop $ServiceName | Out-Host
& $ScExe delete $ServiceName | Out-Host
