param(
  [string]$WinDivertDir = (Join-Path $PSScriptRoot "..\\dist"),
  [string]$ServiceName = "WinDivert",
  [string]$SysName = "",
  [switch]$ForceBinPath = $false
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

function Assert-DriverFileName([string]$Name) {
  if ([string]::IsNullOrWhiteSpace($Name)) {
    return
  }
  if ($Name.Contains('\') -or $Name.Contains('/') -or $Name.Contains(':')) {
    throw "SysName must be a bare .sys filename."
  }
  if ([IO.Path]::GetFileName($Name) -ne $Name) {
    throw "SysName must not include directories."
  }
  if (-not [string]::Equals([IO.Path]::GetExtension($Name), ".sys", [StringComparison]::OrdinalIgnoreCase)) {
    throw "SysName must end with .sys."
  }
  $stem = [IO.Path]::GetFileNameWithoutExtension($Name)
  if ([string]::IsNullOrWhiteSpace($stem) -or $stem -eq "." -or $stem -eq "..") {
    throw "SysName must include a filename before .sys."
  }
  if ($stem.EndsWith(" ") -or $stem.EndsWith(".")) {
    throw "SysName must not end with a space or dot before .sys."
  }
  $upperStem = $stem.Trim().TrimEnd([char[]]@(" ", ".")).ToUpperInvariant()
  $reserved = @("CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$")
  if ($reserved -contains $upperStem -or $upperStem -match '^(COM[1-9]|LPT[1-9])$') {
    throw "SysName must not use a reserved Windows device name."
  }
}

function Assert-SafeDriverDirectory([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
    throw "WinDivertDir not found: $Path"
  }
  if (Test-ReparsePoint $Path) {
    throw "WinDivertDir must not be a reparse point: $Path"
  }
  Assert-NoReparsePath $Path "WinDivertDir"
}

function Assert-SafeDriverFile([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "WinDivert driver sys not found: $Path"
  }
  if (Test-ReparsePoint $Path) {
    throw "WinDivert driver sys must not be a reparse point: $Path"
  }
  Assert-NoReparsePath $Path "WinDivert driver sys"
}

if (-not (Test-Admin)) {
  Write-Error "Administrator privileges required."
  exit 1
}

try {
  Assert-ServiceName $ServiceName
  Assert-DriverFileName $SysName
  $ScExe = Get-System32Command "sc.exe"
} catch {
  Write-Error $_.Exception.Message
  exit 1
}

try {
  Assert-SafeDriverDirectory $WinDivertDir
} catch {
  Write-Error $_.Exception.Message
  exit 1
}

$sysCandidates = @()
if ($SysName -and $SysName.Trim().Length -gt 0) {
  $sysCandidates += $SysName
} else {
  $sysCandidates += "WinDivert64.sys"
  $sysCandidates += "WinDivert.sys"
}

$sysPath = $null
foreach ($candidate in $sysCandidates) {
  $path = Join-Path $WinDivertDir $candidate
  if (Test-Path -LiteralPath $path -PathType Leaf) {
    try {
      Assert-SafeDriverFile $path
    } catch {
      Write-Error $_.Exception.Message
      exit 1
    }
    $sysPath = $path
    break
  }
}

if (-not $sysPath) {
  Write-Error "WinDivert driver sys not found in $WinDivertDir"
  exit 1
}

$sysPath = (Resolve-Path -LiteralPath $sysPath).Path

function Normalize-BinPath([string]$Path) {
  $p = $Path.Trim()
  if ($p.StartsWith('\??\')) {
    $p = $p.Substring(4)
  }
  if ([string]::IsNullOrWhiteSpace($p)) {
    return ""
  }
  if ($p.StartsWith('"')) {
    $trimmed = $p.Substring(1)
    $quote = $trimmed.IndexOf('"')
    if ($quote -ge 0) {
      return $trimmed.Substring(0, $quote).Trim()
    }
    return $trimmed.Trim()
  }

  $parts = $p -split '\s+'
  if ($parts.Count -eq 0) {
    return ""
  }
  $candidate = $parts[0]
  for ($i = 1; $i -lt $parts.Count; $i++) {
    if ([string]::Equals([System.IO.Path]::GetExtension($candidate), ".sys", [System.StringComparison]::OrdinalIgnoreCase)) {
      break
    }
    $candidate = "$candidate $($parts[$i])"
  }
  return $candidate
}

& $ScExe query $ServiceName > $null 2>&1
if ($LASTEXITCODE -ne 0) {
  & $ScExe create $ServiceName type= kernel start= demand binPath= "$sysPath" | Out-Host
} else {
  $qc = & $ScExe qc $ServiceName 2>$null
  $binPath = $null
  foreach ($line in $qc) {
    if ($line -match 'BINARY_PATH_NAME\s*:\s*(.+)$') {
      $binPath = $Matches[1].Trim()
      break
    }
  }

  $normalized = $null
  if ($binPath) {
    $normalized = Normalize-BinPath $binPath
  }

  $needsUpdate = $false
  if (-not $normalized) {
    $needsUpdate = $true
  } elseif (-not (Test-Path -LiteralPath $normalized -PathType Leaf)) {
    $needsUpdate = $true
  } elseif (Test-ReparsePoint $normalized) {
    $needsUpdate = $true
  } else {
    try {
      Assert-NoReparsePath $normalized "existing service bin path"
    } catch {
      $needsUpdate = $true
    }
    if (-not $needsUpdate -and $ForceBinPath -and ($normalized -ne $sysPath)) {
      $needsUpdate = $true
    }
  }

  if ($needsUpdate) {
    $out = & $ScExe config $ServiceName start= demand binPath= "$sysPath" 2>&1
    $out | Out-Host
    if ($LASTEXITCODE -ne 0) {
      $outText = ($out -join "`n")
      if ($outText -match "1072" -or $outText -match "marked for deletion") {
        Write-Error "Service is marked for deletion. Reboot and try again."
      }
      exit 1
    }
  } else {
    & $ScExe config $ServiceName start= demand | Out-Host
  }
}

& $ScExe start $ServiceName | Out-Host
