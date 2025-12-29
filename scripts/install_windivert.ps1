param(
  [string]$WinDivertDir = (Join-Path $PSScriptRoot "..\\dist"),
  [string]$ServiceName = "WinDivert",
  [string]$SysName = "",
  [switch]$ForceBinPath = $false
)

function Test-Admin {
  $id = [Security.Principal.WindowsIdentity]::GetCurrent()
  $p = New-Object Security.Principal.WindowsPrincipal($id)
  return $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Admin)) {
  Write-Error "Administrator privileges required."
  exit 1
}

if (-not (Test-Path $WinDivertDir)) {
  Write-Error "WinDivertDir not found: $WinDivertDir"
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
  if (Test-Path $path) {
    $sysPath = $path
    break
  }
}

if (-not $sysPath) {
  Write-Error "WinDivert driver sys not found in $WinDivertDir"
  exit 1
}

$sysPath = (Resolve-Path $sysPath).Path

function Normalize-BinPath([string]$Path) {
  $p = $Path.Trim()
  if ($p.StartsWith('"') -and $p.EndsWith('"')) {
    $p = $p.Trim('"')
  }
  if ($p.StartsWith('\??\')) {
    $p = $p.Substring(4)
  }
  $lower = $p.ToLowerInvariant()
  $idx = $lower.IndexOf('.sys')
  if ($idx -ge 0) {
    return $p.Substring(0, $idx + 4)
  }
  $space = $p.IndexOf(' ')
  if ($space -gt 0) {
    return $p.Substring(0, $space)
  }
  return $p
}

sc.exe query $ServiceName > $null 2>&1
if ($LASTEXITCODE -ne 0) {
  sc.exe create $ServiceName type= kernel start= demand binPath= "$sysPath" | Out-Host
} else {
  $qc = sc.exe qc $ServiceName 2>$null
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
  } elseif (-not (Test-Path $normalized)) {
    $needsUpdate = $true
  } elseif ($ForceBinPath -and ($normalized -ne $sysPath)) {
    $needsUpdate = $true
  }

  if ($needsUpdate) {
    $out = sc.exe config $ServiceName start= demand binPath= "$sysPath" | Out-Host
    if ($LASTEXITCODE -ne 0) {
      if ($out -match "1072" -or $out -match "marked for deletion") {
        Write-Error "Service is marked for deletion. Reboot and try again."
      }
      exit 1
    }
  } else {
    sc.exe config $ServiceName start= demand | Out-Host
  }
}

sc.exe start $ServiceName | Out-Host
