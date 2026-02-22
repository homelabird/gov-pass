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

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$licenseSrc = Join-Path $repoRoot "LICENSE"
$noticesSrc = Join-Path $repoRoot "docs\\THIRD_PARTY_NOTICES.md"
if (-not (Test-Path $licenseSrc)) {
  Write-Error "LICENSE not found: $licenseSrc"
  exit 1
}
if (-not (Test-Path $noticesSrc)) {
  Write-Error "THIRD_PARTY_NOTICES.md not found: $noticesSrc"
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

Copy-Item -Force $licenseSrc (Join-Path $OutDir "LICENSE")
$docsOut = Join-Path $OutDir "docs"
New-Item -ItemType Directory -Force -Path $docsOut | Out-Null
Copy-Item -Force $noticesSrc (Join-Path $docsOut "THIRD_PARTY_NOTICES.md")

$licensesOut = Join-Path $OutDir "licenses"
New-Item -ItemType Directory -Force -Path $licensesOut | Out-Null
$licenseFiles = @(
  @{ Src = Join-Path $repoRoot "third_party\\windivert\\WinDivert-2.2.2-A\\LICENSE"; Dest = Join-Path $licensesOut "WinDivert-LICENSE.txt" },
  @{ Src = Join-Path $repoRoot "third_party\\go-nfqueue\\LICENSE"; Dest = Join-Path $licensesOut "go-nfqueue-LICENSE.txt" },
  @{ Src = Join-Path $repoRoot "third_party\\netlink\\LICENSE.md"; Dest = Join-Path $licensesOut "netlink-LICENSE.txt" }
)
foreach ($item in $licenseFiles) {
  if (Test-Path $item.Src) {
    Copy-Item -Force $item.Src $item.Dest
  }
}

Write-Host "Packaged to $OutDir"
