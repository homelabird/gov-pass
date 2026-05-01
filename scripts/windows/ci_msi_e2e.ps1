param(
  [string]$MsiPath = "",
  [switch]$PurgeProgramData,
  [switch]$RemoveWinDivert
)

$ErrorActionPreference = "Stop"

function Test-IsAdmin {
  $id = [Security.Principal.WindowsIdentity]::GetCurrent()
  $p = New-Object Security.Principal.WindowsPrincipal($id)
  return $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-System32Command {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name
  )

  if ([IO.Path]::GetFileName($Name) -ne $Name) {
    throw "System32 command name must be a bare filename: $Name"
  }
  $sysDir = [Environment]::SystemDirectory
  if ([string]::IsNullOrWhiteSpace($sysDir)) {
    throw "Unable to resolve System32"
  }
  $path = Join-Path $sysDir $Name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "System32 command not found: $path"
  }
  Assert-NoReparsePath -Path $path -Label "System32 command"
  return $path
}

function Test-ReparsePoint {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path
  )
  if (-not (Test-Path -LiteralPath $Path)) {
    return $false
  }
  $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
  return (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)
}

function Assert-NoReparsePoint {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  if ((Test-Path -LiteralPath $Path) -and (Test-ReparsePoint -Path $Path)) {
    throw "$Label must not be a reparse point: $Path"
  }
}

function Assert-NoReparsePath {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )

  $current = [IO.Path]::GetFullPath($Path)
  while (-not [string]::IsNullOrWhiteSpace($current)) {
    if (Test-Path -LiteralPath $current) {
      Assert-NoReparsePoint -Path $current -Label "$Label path component"
    }
    $parent = Split-Path -Parent $current
    if ([string]::IsNullOrWhiteSpace($parent) -or $parent -eq $current) {
      break
    }
    $current = $parent
  }
}

function Assert-ParentNoReparsePoint {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  $parent = Split-Path -Parent $Path
  if (-not [string]::IsNullOrWhiteSpace($parent)) {
    Assert-NoReparsePath -Path $parent -Label "$Label parent"
  }
}

function Remove-SafeFileIfPresent {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  Assert-ParentNoReparsePoint -Path $Path -Label $Label
  if (-not (Test-Path -LiteralPath $Path)) {
    return
  }
  Assert-NoReparsePoint -Path $Path -Label $Label
  if (Test-Path -LiteralPath $Path -PathType Container) {
    throw "$Label must not be a directory: $Path"
  }
  Remove-Item -Force -LiteralPath $Path
}

function Get-SafeTextFile {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  Assert-NoReparsePath -Path $Path -Label $Label
  return Get-Content -Raw -LiteralPath $Path
}

function Set-SafeTextFile {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$Value,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  Assert-ParentNoReparsePoint -Path $Path -Label $Label
  if (Test-Path -LiteralPath $Path) {
    Assert-NoReparsePoint -Path $Path -Label $Label
    if (Test-Path -LiteralPath $Path -PathType Container) {
      throw "$Label must not be a directory: $Path"
    }
  }
  Set-Content -Encoding ASCII -LiteralPath $Path -Value $Value
}

if (-not (Test-IsAdmin)) {
  throw "Administrator privileges are required to install/uninstall MSI and manage services in CI."
}

$MsiExecPath = Get-System32Command "msiexec.exe"
$ScExePath = Get-System32Command "sc.exe"
$commonAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
if ([string]::IsNullOrWhiteSpace($commonAppData)) {
  $commonAppData = "C:\ProgramData"
}
$programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
if ([string]::IsNullOrWhiteSpace($programFiles)) {
  $programFiles = "C:\Program Files"
}
Assert-NoReparsePath -Path $commonAppData -Label "CommonApplicationData"
Assert-NoReparsePath -Path $programFiles -Label "ProgramFiles"

$projectDir = $env:CI_PROJECT_DIR
if ([string]::IsNullOrWhiteSpace($projectDir)) {
  $projectDir = (Get-Location).Path
}

if ([string]::IsNullOrWhiteSpace($MsiPath)) {
  $msi = Get-ChildItem -Path (Join-Path $projectDir "dist\\release") -Filter "*-windows-amd64.msi" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
  if (-not $msi) {
    throw "MSI not found under dist\\release\\ (filter: *-windows-amd64.msi)"
  }
  $MsiPath = $msi.FullName
}

Write-Host "MSI: $MsiPath"

function Invoke-MsiExec {
  param(
    [string[]]$Args,
    [int[]]$OkExitCodes = @(0, 3010)
  )
  $p = Start-Process -FilePath $script:MsiExecPath -ArgumentList $Args -Wait -PassThru
  if ($OkExitCodes -notcontains $p.ExitCode) {
    throw "msiexec $($Args -join ' ') failed with exit code $($p.ExitCode)"
  }
  return $p.ExitCode
}

$svcName = "gov-pass"

function Wait-ServiceRunning {
  param(
    [string]$Name,
    [int]$TimeoutSeconds = 60
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $svc = Get-Service -Name $Name -ErrorAction Stop
      if ($svc.Status -eq [System.ServiceProcess.ServiceControllerStatus]::Running) {
        return $svc
      }
    } catch {
      # not installed yet
    }
    Start-Sleep -Seconds 2
  }
  throw "Service $Name did not reach Running within ${TimeoutSeconds}s"
}

function Wait-ServiceStatus {
  param(
    [string]$Name,
    [string]$Status,
    [int]$TimeoutSeconds = 60
  )
  $desired = [System.ServiceProcess.ServiceControllerStatus]::$Status
  $svc = Get-Service -Name $Name -ErrorAction Stop
  $svc.WaitForStatus($desired, [TimeSpan]::FromSeconds($TimeoutSeconds))
  $svc.Refresh()
  return $svc
}

function Wait-PathExists {
  param(
    [string]$Path,
    [int]$TimeoutSeconds = 30
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    if (Test-Path -LiteralPath $Path) {
      Assert-NoReparsePath -Path $Path -Label "wait path"
      return
    }
    Start-Sleep -Seconds 1
  }
  throw "Path not found within ${TimeoutSeconds}s: $Path"
}

function Wait-PathMissing {
  param(
    [string]$Path,
    [int]$TimeoutSeconds = 30
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    if (-not (Test-Path -LiteralPath $Path)) {
      return
    }
    Start-Sleep -Seconds 1
  }
  throw "Path still exists after ${TimeoutSeconds}s: $Path"
}

function Wait-LogMatch {
  param(
    [string]$Path,
    [string]$Pattern,
    [int]$TimeoutSeconds = 30
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    if (Test-Path -LiteralPath $Path) {
      $content = Get-SafeTextFile -Path $Path -Label "service log"
      if ($content -match $Pattern) {
        return $content
      }
    }
    Start-Sleep -Seconds 1
  }
  throw "Log pattern not found within ${TimeoutSeconds}s: $Pattern"
}

function Wait-ServiceMissing {
  param(
    [string]$Name,
    [int]$TimeoutSeconds = 60
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      Get-Service -Name $Name -ErrorAction Stop | Out-Null
      Start-Sleep -Seconds 2
      continue
    } catch {
      return
    }
  }
  throw "Service $Name still exists after ${TimeoutSeconds}s"
}

function Invoke-NativeCapture {
  param(
    [Parameter(Mandatory = $true)]
    [string]$FilePath,
    [string[]]$ArgumentList = @(),
    [string]$InputText = ""
  )

  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = $FilePath
  $psi.UseShellExecute = $false
  $psi.RedirectStandardOutput = $true
  $psi.RedirectStandardError = $true
  $psi.RedirectStandardInput = $true
  foreach ($arg in $ArgumentList) {
    [void]$psi.ArgumentList.Add($arg)
  }

  $proc = New-Object System.Diagnostics.Process
  $proc.StartInfo = $psi
  [void]$proc.Start()
  if (-not [string]::IsNullOrEmpty($InputText)) {
    $proc.StandardInput.Write($InputText)
  }
  $proc.StandardInput.Close()

  $stdout = $proc.StandardOutput.ReadToEnd()
  $stderr = $proc.StandardError.ReadToEnd()
  $proc.WaitForExit()

  [pscustomobject]@{
    ExitCode = $proc.ExitCode
    Stdout   = $stdout
    Stderr   = $stderr
    Output   = $stdout + $stderr
  }
}

$programDataDir = Join-Path $commonAppData "gov-pass"
$cfgPath = Join-Path $programDataDir "config.json"
$logPath = Join-Path $programDataDir "splitter.log"

$runningInCi = ($env:CI -eq "true")
$wantPurgeProgramData = $PurgeProgramData.IsPresent -or $runningInCi
$wantRemoveWinDivert = $RemoveWinDivert.IsPresent

function Assert-AuthenticodeSigned {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path
  )

  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "Signature check failed: file not found: $Path"
  }
  Assert-NoReparsePath -Path $Path -Label "signed file"

  $sig = Get-AuthenticodeSignature -FilePath $Path
  if (-not $sig) {
    throw "Signature check failed: Get-AuthenticodeSignature returned null: $Path"
  }

  if ($sig.Status -eq "NotSigned" -or $sig.SignatureType -eq "None" -or -not $sig.SignerCertificate) {
    throw "Expected Authenticode signature but file is not signed: $Path (Status=$($sig.Status))"
  }
  if ($sig.Status -eq "HashMismatch") {
    throw "Authenticode signature hash mismatch: $Path"
  }
  if ($sig.Status -ne "Valid") {
    Write-Host "warning: signature status for ${Path}: $($sig.Status) - $($sig.StatusMessage)"
  }
}

$installed = $false
$tuiExePath = ""

# Require signed MSI/EXEs in Windows MSI CI.
Assert-AuthenticodeSigned -Path $MsiPath

try {
  # Best-effort uninstall any previous install matching this package (1605 = not installed).
  Invoke-MsiExec -Args @("/x", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010, 1605) | Out-Null

  # Clean up ProgramData residue from previous runs so log assertions are stable.
  try {
    Stop-Service -Name $svcName -ErrorAction SilentlyContinue
  } catch {
    # ignore
  }
  Remove-SafeFileIfPresent -Path $cfgPath -Label "config"
  Remove-SafeFileIfPresent -Path $logPath -Label "service log"

  # Install.
  Invoke-MsiExec -Args @("/i", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010) | Out-Null
  $installed = $true

  $svc = Wait-ServiceRunning -Name $svcName -TimeoutSeconds 60

  $installDir = Join-Path $programFiles "gov-pass"
  $exePath = Join-Path $installDir "splitter.exe"
  if (-not (Test-Path -LiteralPath $exePath -PathType Leaf)) {
    throw "splitter.exe not found: $exePath"
  }
  Assert-AuthenticodeSigned -Path $exePath
  $tuiExePath = Join-Path $installDir "gov-pass-tui.exe"
  if (-not (Test-Path -LiteralPath $tuiExePath -PathType Leaf)) {
    throw "gov-pass-tui.exe not found: $tuiExePath"
  }
  Assert-AuthenticodeSigned -Path $tuiExePath
  $helperExePath = Join-Path $installDir "gov-pass-msi-helper.exe"
  if (-not (Test-Path -LiteralPath $helperExePath -PathType Leaf)) {
    throw "gov-pass-msi-helper.exe not found: $helperExePath"
  }
  Assert-AuthenticodeSigned -Path $helperExePath

  $schemaPath = Join-Path $installDir "docs\\schema\\splitter.windows.schema.json"
  $examplePath = Join-Path $installDir "docs\\examples\\splitter.windows.json"
  Wait-PathExists -Path $schemaPath -TimeoutSeconds 30
  Wait-PathExists -Path $examplePath -TimeoutSeconds 30
  Get-SafeTextFile -Path $schemaPath -Label "schema" | ConvertFrom-Json | Out-Null
  Get-SafeTextFile -Path $examplePath -Label "example config" | ConvertFrom-Json | Out-Null

  $menuDir = Join-Path $commonAppData "Microsoft\\Windows\\Start Menu\\Programs\\gov-pass"
  $lnkTui = Join-Path $menuDir "gov-pass TUI.lnk"
  $lnkStart = Join-Path $menuDir "Start gov-pass service (Admin).lnk"
  $lnkStop = Join-Path $menuDir "Stop gov-pass service (Admin).lnk"
  $lnkReload = Join-Path $menuDir "Reload gov-pass config (Admin).lnk"
  Wait-PathExists -Path $lnkTui -TimeoutSeconds 30
  Wait-PathExists -Path $lnkStart -TimeoutSeconds 30
  Wait-PathExists -Path $lnkStop -TimeoutSeconds 30
  Wait-PathExists -Path $lnkReload -TimeoutSeconds 30

  Wait-PathExists -Path $cfgPath -TimeoutSeconds 30
  Wait-PathExists -Path $logPath -TimeoutSeconds 30

  # Mutate config so reload is observable.
  $cfg = Get-SafeTextFile -Path $cfgPath -Label "config" | ConvertFrom-Json
  if (-not $cfg.engine) {
    throw "config.json missing engine section"
  }
  if ($null -eq $cfg.engine.split_chunk) {
    throw "config.json missing engine.split_chunk"
  }
  if (-not $cfg.windivert) {
    throw "config.json missing windivert section"
  }
  if ([string]::IsNullOrWhiteSpace([string]$cfg.windivert.filter)) {
    throw "config.json missing windivert.filter"
  }
  $oldChunk = [int]$cfg.engine.split_chunk
  $newChunk = $oldChunk + 1
  $cfg.engine.split_chunk = $newChunk
  Set-SafeTextFile -Path $cfgPath -Value (($cfg | ConvertTo-Json -Depth 16) + "`n") -Label "config"

  # Reload.
  & $ScExePath control $svcName paramchange | Out-Host
  Start-Sleep -Seconds 2

  $svc = Get-Service -Name $svcName -ErrorAction Stop
  $svc.Refresh()
  if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Running) {
    throw "Service not running after reload (status: $($svc.Status))"
  }

  # Confirm we applied config without restarting the engine loop.
  $log = Wait-LogMatch -Path $logPath -Pattern ([regex]::Escape("split_chunk=$newChunk")) -TimeoutSeconds 30
  $engineStartedCount = ([regex]::Matches($log, "engine started \\(workers=")).Count
  if ($engineStartedCount -lt 1) {
    throw "Expected 'engine started' log line not found"
  }
  if ($engineStartedCount -gt 1) {
    throw "Engine appears to have restarted during reload (engine started count=$engineStartedCount)"
  }

  # Mutate filter + queue defaults so service reload must reopen the WinDivert handle.
  $oldFilter = [string]$cfg.windivert.filter
  $newFilter = if ($oldFilter -match "8443") {
    "outbound and (ip or ipv6) and tcp.DstPort == 443"
  } elseif ($oldFilter -match "443") {
    $oldFilter -replace "443", "8443"
  } else {
    "outbound and (ip or ipv6) and tcp.DstPort == 8443"
  }
  $cfg.windivert.filter = $newFilter
  $cfg.windivert.queue_len = 0
  $cfg.windivert.queue_time_ms = 0
  $cfg.windivert.queue_size_bytes = 0
  Set-SafeTextFile -Path $cfgPath -Value (($cfg | ConvertTo-Json -Depth 16) + "`n") -Label "config"

  & $ScExePath control $svcName paramchange | Out-Host
  Start-Sleep -Seconds 2

  $svc = Get-Service -Name $svcName -ErrorAction Stop
  $svc.Refresh()
  if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Running) {
    throw "Service not running after WinDivert reload (status: $($svc.Status))"
  }

  $log = Wait-LogMatch -Path $logPath -Pattern ([regex]::Escape("reload: WinDivert handle reopened")) -TimeoutSeconds 30
  $engineStartedCountAfterReopen = ([regex]::Matches($log, "engine started \\(workers=")).Count
  if ($engineStartedCountAfterReopen -ne $engineStartedCount) {
    throw "Engine appears to have restarted during WinDivert reload (before=$engineStartedCount after=$engineStartedCountAfterReopen)"
  }
  if ($log -notmatch [regex]::Escape($newFilter)) {
    throw "WinDivert reopen log missing updated filter: $newFilter"
  }
  if ($log -notmatch "queue_len=0") {
    throw "WinDivert reopen log missing queue_len=0"
  }
  if ($log -notmatch "queue_time_ms=0") {
    throw "WinDivert reopen log missing queue_time_ms=0"
  }
  if ($log -notmatch "queue_size_bytes=0") {
    throw "WinDivert reopen log missing queue_size_bytes=0"
  }

  # Stop/Start smoke.
  Stop-Service -Name $svcName -ErrorAction Stop
  $svc = Wait-ServiceStatus -Name $svcName -Status "Stopped" -TimeoutSeconds 60
  Start-Service -Name $svcName -ErrorAction Stop
  $svc = Wait-ServiceStatus -Name $svcName -Status "Running" -TimeoutSeconds 60

  # TUI smoke.
  $tuiStatus = Invoke-NativeCapture -FilePath $tuiExePath -ArgumentList @("--service-name", $svcName, "--action", "status")
  if ($tuiStatus.ExitCode -ne 0) {
    throw "TUI status smoke failed with exit code $($tuiStatus.ExitCode): $($tuiStatus.Output)"
  }
  if ($tuiStatus.Output -notmatch "active|inactive") {
    throw "TUI status smoke returned unexpected output: $($tuiStatus.Output)"
  }

  $tuiReload = Invoke-NativeCapture -FilePath $tuiExePath -ArgumentList @("--service-name", $svcName, "--action", "reload")
  if ($tuiReload.ExitCode -ne 0) {
    throw "TUI reload smoke failed with exit code $($tuiReload.ExitCode): $($tuiReload.Output)"
  }

  $tuiPanel = Invoke-NativeCapture -FilePath $tuiExePath -ArgumentList @("--service-name", $svcName) -InputText "r`nq`n"
  if ($tuiPanel.ExitCode -ne 0) {
    throw "Interactive TUI smoke failed with exit code $($tuiPanel.ExitCode): $($tuiPanel.Output)"
  }
  if ($tuiPanel.Output -notmatch "GOV-PASS CONTROL") {
    throw "Interactive TUI smoke missing panel header: $($tuiPanel.Output)"
  }
  if ($tuiPanel.Output -notmatch "Select>") {
    throw "Interactive TUI smoke missing prompt: $($tuiPanel.Output)"
  }

} finally {
  # Best-effort uninstall.
  try {
    $args = @("/x", $MsiPath, "/qn", "/norestart")
    if ($wantPurgeProgramData) {
      $args += "GOVPASS_PURGE_PROGRAMDATA=1"
    }
    if ($wantRemoveWinDivert) {
      $args += "GOVPASS_REMOVE_WINDIVERT=1"
    }
    Invoke-MsiExec -Args $args -OkExitCodes @(0, 3010, 1605) | Out-Null
  } catch {
    Write-Host "warning: uninstall failed: $($_.Exception.Message)"
  }
  try {
    Wait-ServiceMissing -Name $svcName -TimeoutSeconds 60
  } catch {
    Write-Host "warning: service removal check failed: $($_.Exception.Message)"
  }

  if ($installed -and $wantPurgeProgramData) {
    Wait-PathMissing -Path $programDataDir -TimeoutSeconds 30
  }
}

Write-Host "MSI service e2e verification passed."
