param(
  [switch]$SkipDriverInstall = $false
)

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$splitterExe = Join-Path $repoRoot "dist\splitter.exe"
$windivertDir = Join-Path $repoRoot "third_party\windivert\WinDivert-2.2.2-A\x64"

Push-Location $repoRoot
try {
  go build -o $splitterExe .\cmd\splitter
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  & (Join-Path $repoRoot "scripts\package.ps1") -ExePath $splitterExe -WinDivertDir $windivertDir -OutDir (Join-Path $repoRoot "dist")
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  if (-not $SkipDriverInstall) {
    & (Join-Path $repoRoot "scripts\install_windivert.ps1") -WinDivertDir (Join-Path $repoRoot "dist")
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
