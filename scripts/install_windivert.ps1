param(
  [string]$WinDivertDir = (Join-Path $PSScriptRoot "..\\dist"),
  [string]$ServiceName = "WinDivert",
  [string]$SysName = ""
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

sc.exe query $ServiceName > $null 2>&1
if ($LASTEXITCODE -ne 0) {
  sc.exe create $ServiceName type= kernel start= demand binPath= "$sysPath" | Out-Host
}

sc.exe start $ServiceName | Out-Host
