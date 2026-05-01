param(
  [Parameter(Mandatory=$true)]
  [string]$ExePath,
  [Parameter(Mandatory=$true)]
  [string]$WinDivertDir,
  [string]$OutDir = (Join-Path $PSScriptRoot "..\\dist")
)

$ErrorActionPreference = "Stop"

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

function Assert-ParentNoReparsePath([string]$Path, [string]$Label) {
  $parent = Split-Path -Parent $Path
  if (-not [string]::IsNullOrWhiteSpace($parent)) {
    Assert-NoReparsePath $parent "$Label parent"
  }
}

function New-SafeDirectory([string]$Path, [string]$Label) {
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

function Assert-SafeSourceFile([string]$Path, [string]$Label) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "$Label source not found: $Path"
  }
  Assert-NoReparsePath $Path "$Label source"
}

function Assert-SafeSourceDirectory([string]$Path, [string]$Label) {
  if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
    throw "$Label source directory not found: $Path"
  }
  Assert-NoReparsePath $Path "$Label source directory"
}

function Copy-SafeFile([string]$Source, [string]$Destination, [string]$Label) {
  Assert-SafeSourceFile $Source $Label
  Assert-ParentNoReparsePath $Destination "$Label destination"
  if (Test-Path -LiteralPath $Destination) {
    Assert-NoReparsePath $Destination "$Label destination"
    if (Test-Path -LiteralPath $Destination -PathType Container) {
      throw "$Label destination is a directory: $Destination"
    }
  }
  Copy-Item -Force -LiteralPath $Source -Destination $Destination
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$licenseSrc = Join-Path $repoRoot "LICENSE"
$noticesSrc = Join-Path $repoRoot "docs\\THIRD_PARTY_NOTICES.md"
$schemaSrc = Join-Path $repoRoot "docs\\schema"
$examplesSrc = Join-Path $repoRoot "docs\\examples"

try {
  Assert-SafeSourceFile $ExePath "ExePath"
  Assert-SafeSourceDirectory $WinDivertDir "WinDivertDir"
  Assert-SafeSourceFile $licenseSrc "LICENSE"
  Assert-SafeSourceFile $noticesSrc "THIRD_PARTY_NOTICES.md"
  Assert-SafeSourceDirectory $schemaSrc "schema docs"
  Assert-SafeSourceDirectory $examplesSrc "example configs"
} catch {
  Write-Error $_.Exception.Message
  exit 1
}

try {
  New-SafeDirectory $OutDir "OutDir"
} catch {
  Write-Error $_.Exception.Message
  exit 1
}

$exeName = Split-Path -Leaf $ExePath
$destExe = Join-Path $OutDir $exeName
$srcExe = (Resolve-Path -LiteralPath $ExePath).Path
$dstExe = $null
if (Test-Path -LiteralPath $destExe) {
  Assert-NoReparsePath $destExe "splitter exe destination"
  if (Test-Path -LiteralPath $destExe -PathType Container) {
    throw "splitter exe destination is a directory: $destExe"
  }
  $dstExe = (Resolve-Path -LiteralPath $destExe -ErrorAction Stop).Path
}
if (-not $dstExe -or $dstExe -ne $srcExe) {
  Copy-SafeFile $ExePath $destExe "splitter exe"
}

$files = @("WinDivert.dll", "WinDivert64.sys", "WinDivert.sys", "WinDivert.cat")
$copiedSys = $false
foreach ($file in $files) {
  $src = Join-Path $WinDivertDir $file
  if (Test-Path -LiteralPath $src -PathType Leaf) {
    Copy-SafeFile $src (Join-Path $OutDir $file) $file
    if ($file -like "*.sys") {
      $copiedSys = $true
    }
  }
}

if (-not $copiedSys) {
  Write-Error "No WinDivert .sys found in $WinDivertDir"
  exit 1
}

Copy-SafeFile $licenseSrc (Join-Path $OutDir "LICENSE") "LICENSE"
$docsOut = Join-Path $OutDir "docs"
New-SafeDirectory $docsOut "docs output directory"
Copy-SafeFile $noticesSrc (Join-Path $docsOut "THIRD_PARTY_NOTICES.md") "THIRD_PARTY_NOTICES.md"
Copy-SafeFile (Join-Path $repoRoot "README.md") (Join-Path $OutDir "README.md") "README.md"
Copy-SafeFile (Join-Path $repoRoot "SECURITY.md") (Join-Path $OutDir "SECURITY.md") "SECURITY.md"
Copy-SafeFile (Join-Path $repoRoot "docs\\DESIGN.md") (Join-Path $docsOut "DESIGN.md") "DESIGN.md"

$schemaOut = Join-Path $docsOut "schema"
$examplesOut = Join-Path $docsOut "examples"
New-SafeDirectory $schemaOut "schema output directory"
New-SafeDirectory $examplesOut "examples output directory"
Get-ChildItem -LiteralPath $schemaSrc -File | ForEach-Object {
  Copy-SafeFile $_.FullName (Join-Path $schemaOut $_.Name) $_.Name
}
Get-ChildItem -LiteralPath $examplesSrc -Filter "splitter.*.json" -File | ForEach-Object {
  Copy-SafeFile $_.FullName (Join-Path $examplesOut $_.Name) $_.Name
}

$licensesOut = Join-Path $OutDir "licenses"
New-SafeDirectory $licensesOut "licenses output directory"
$licenseFiles = @(
  @{ Src = Join-Path $repoRoot "third_party\\windivert\\WinDivert-2.2.2-A\\LICENSE"; Dest = Join-Path $licensesOut "WinDivert-LICENSE.txt" },
  @{ Src = Join-Path $repoRoot "third_party\\go-nfqueue\\LICENSE"; Dest = Join-Path $licensesOut "go-nfqueue-LICENSE.txt" },
  @{ Src = Join-Path $repoRoot "third_party\\netlink\\LICENSE.md"; Dest = Join-Path $licensesOut "netlink-LICENSE.txt" }
)
foreach ($item in $licenseFiles) {
  if (Test-Path -LiteralPath $item.Src -PathType Leaf) {
    Copy-SafeFile $item.Src $item.Dest (Split-Path -Leaf $item.Dest)
  }
}

Write-Host "Packaged to $OutDir"
