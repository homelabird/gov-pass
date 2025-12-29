param(
  [Parameter(Mandatory=$true)]
  [string]$ExePath,
  [Parameter(Mandatory=$true)]
  [string]$WinDivertDir,
  [string]$OutDir = (Join-Path $PSScriptRoot "..\\dist")
)

if (-not (Test-Path $ExePath)) {
  Write-Error "ExePath not found: $ExePath"
  exit 1
}

if (-not (Test-Path $WinDivertDir)) {
  Write-Error "WinDivertDir not found: $WinDivertDir"
  exit 1
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$exeName = Split-Path -Leaf $ExePath
$destExe = Join-Path $OutDir $exeName
$srcExe = (Resolve-Path $ExePath).Path
$dstExe = $null
try {
  $dstExe = (Resolve-Path -LiteralPath $destExe -ErrorAction Stop).Path
} catch {
  $dstExe = $null
}
if (-not $dstExe -or $dstExe -ne $srcExe) {
  Copy-Item -Force $ExePath $destExe
}

$files = @("WinDivert.dll", "WinDivert64.sys", "WinDivert.sys", "WinDivert.cat")
$copiedSys = $false
foreach ($file in $files) {
  $src = Join-Path $WinDivertDir $file
  if (Test-Path $src) {
    Copy-Item -Force $src (Join-Path $OutDir $file)
    if ($file -like "*.sys") {
      $copiedSys = $true
    }
  }
}

if (-not $copiedSys) {
  Write-Error "No WinDivert .sys found in $WinDivertDir"
  exit 1
}

Write-Host "Packaged to $OutDir"
