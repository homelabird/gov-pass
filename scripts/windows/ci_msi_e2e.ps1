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

if (-not (Test-IsAdmin)) {
  throw "Administrator privileges are required to install/uninstall MSI and manage services in CI."
}

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
  $p = Start-Process -FilePath "msiexec.exe" -ArgumentList $Args -Wait -PassThru
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
    if (Test-Path $Path) {
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
    if (-not (Test-Path $Path)) {
      return
    }
    Start-Sleep -Seconds 1
  }
  throw "Path still exists after ${TimeoutSeconds}s: $Path"
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

$programDataDir = "C:\\ProgramData\\gov-pass"
$cfgPath = Join-Path $programDataDir "config.json"
$logPath = Join-Path $programDataDir "splitter.log"
$runKeyHKCU = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
$runKeyHKLM = "HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
$runValueNameTray = "gov-pass-tray"

$runningInCi = ($env:CI -eq "true")
$wantPurgeProgramData = $PurgeProgramData.IsPresent -or $runningInCi
$wantRemoveWinDivert = $RemoveWinDivert.IsPresent

function Get-RunValue {
  param(
    [string]$Hive, # HKCU or HKLM
    [string]$Name
  )
  $psPath = "${Hive}:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
  try {
    $item = Get-ItemProperty -Path $psPath -Name $Name -ErrorAction Stop
    return @{ Exists = $true; Value = [string]$item.$Name }
  } catch {
    return @{ Exists = $false; Value = "" }
  }
}

function Set-RunValue {
  param(
    [string]$Hive, # HKCU or HKLM
    [string]$Name,
    [string]$Value
  )
  $psPath = "${Hive}:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
  New-Item -Path $psPath -Force | Out-Null
  New-ItemProperty -Path $psPath -Name $Name -Value $Value -PropertyType String -Force | Out-Null
}

function Remove-RunValueBestEffort {
  param(
    [string]$Hive, # HKCU or HKLM
    [string]$Name
  )
  $psPath = "${Hive}:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
  try {
    Remove-ItemProperty -Path $psPath -Name $Name -ErrorAction SilentlyContinue
  } catch {
    # ignore
  }
}

function Assert-RunValueMissing {
  param(
    [string]$Key,
    [string]$Name
  )
  & reg.exe query $Key /v $Name | Out-Null
  if ($LASTEXITCODE -eq 0) {
    throw "Registry autorun value still exists: $Key\\$Name"
  }
}

$installed = $false
$trayExePath = ""
$prevHKCU = @{ Exists = $false; Value = "" }
$prevHKLM = @{ Exists = $false; Value = "" }
$regTouched = $false

try {
  # Best-effort uninstall any previous install matching this package (1605 = not installed).
  Invoke-MsiExec -Args @("/x", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010, 1605) | Out-Null

  # Clean up ProgramData residue from previous runs so log assertions are stable.
  try {
    Stop-Service -Name $svcName -ErrorAction SilentlyContinue
  } catch {
    # ignore
  }
  try { Remove-Item -Force -ErrorAction SilentlyContinue $cfgPath } catch { }
  try { Remove-Item -Force -ErrorAction SilentlyContinue $logPath } catch { }

  # Install.
  Invoke-MsiExec -Args @("/i", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010) | Out-Null
  $installed = $true

  $svc = Wait-ServiceRunning -Name $svcName -TimeoutSeconds 60

  $installDir = Join-Path $env:ProgramFiles "gov-pass"
  $exePath = Join-Path $installDir "splitter.exe"
  if (-not (Test-Path $exePath)) {
    throw "splitter.exe not found: $exePath"
  }
  $trayExePath = Join-Path $installDir "gov-pass-tray.exe"
  if (-not (Test-Path $trayExePath)) {
    throw "gov-pass-tray.exe not found: $trayExePath"
  }
  $helperExePath = Join-Path $installDir "gov-pass-msi-helper.exe"
  if (-not (Test-Path $helperExePath)) {
    throw "gov-pass-msi-helper.exe not found: $helperExePath"
  }

  $menuDir = Join-Path $env:ProgramData "Microsoft\\Windows\\Start Menu\\Programs\\gov-pass"
  $lnkTray = Join-Path $menuDir "gov-pass tray.lnk"
  $lnkStart = Join-Path $menuDir "Start gov-pass service (Admin).lnk"
  $lnkStop = Join-Path $menuDir "Stop gov-pass service (Admin).lnk"
  $lnkReload = Join-Path $menuDir "Reload gov-pass config (Admin).lnk"
  Wait-PathExists -Path $lnkTray -TimeoutSeconds 30
  Wait-PathExists -Path $lnkStart -TimeoutSeconds 30
  Wait-PathExists -Path $lnkStop -TimeoutSeconds 30
  Wait-PathExists -Path $lnkReload -TimeoutSeconds 30

  Wait-PathExists -Path $cfgPath -TimeoutSeconds 30
  Wait-PathExists -Path $logPath -TimeoutSeconds 30

  # Mutate config so reload is observable.
  $cfg = Get-Content -Raw -Path $cfgPath | ConvertFrom-Json
  if (-not $cfg.engine) {
    throw "config.json missing engine section"
  }
  if (-not $cfg.engine.split_chunk) {
    throw "config.json missing engine.split_chunk"
  }
  $oldChunk = [int]$cfg.engine.split_chunk
  $newChunk = $oldChunk + 1
  $cfg.engine.split_chunk = $newChunk
  ($cfg | ConvertTo-Json -Depth 16) + "`n" | Set-Content -Encoding ASCII -Path $cfgPath

  # Reload.
  & sc.exe control $svcName paramchange | Out-Host
  Start-Sleep -Seconds 2

  $svc = Get-Service -Name $svcName -ErrorAction Stop
  $svc.Refresh()
  if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Running) {
    throw "Service not running after reload (status: $($svc.Status))"
  }

  # Confirm we applied config without restarting the engine loop.
  $log = Get-Content -Raw -Path $logPath
  $engineStartedCount = ([regex]::Matches($log, "engine started \\(workers=")).Count
  if ($engineStartedCount -lt 1) {
    throw "Expected 'engine started' log line not found"
  }
  if ($engineStartedCount -gt 1) {
    throw "Engine appears to have restarted during reload (engine started count=$engineStartedCount)"
  }
  if ($log -notmatch "split_chunk=$newChunk") {
    throw "Reload did not log expected split_chunk=$newChunk"
  }

  # Stop/Start smoke.
  Stop-Service -Name $svcName -ErrorAction Stop
  $svc = Wait-ServiceStatus -Name $svcName -Status "Stopped" -TimeoutSeconds 60
  Start-Service -Name $svcName -ErrorAction Stop
  $svc = Wait-ServiceStatus -Name $svcName -Status "Running" -TimeoutSeconds 60

  # Create fake autorun entries so MSI uninstall cleanup is verifiable.
  # We restore previous values afterwards (if they existed) to avoid side effects.
  $prevHKCU = Get-RunValue -Hive "HKCU" -Name $runValueNameTray
  $prevHKLM = Get-RunValue -Hive "HKLM" -Name $runValueNameTray
  Set-RunValue -Hive "HKCU" -Name $runValueNameTray -Value $trayExePath
  Set-RunValue -Hive "HKLM" -Name $runValueNameTray -Value $trayExePath
  $regTouched = $true
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

  if ($installed) {
    # Uninstall should clean up the tray autorun Run value (best-effort for both HKCU/HKLM).
    # If this fails, a stale autorun entry will remain on the machine.
    Assert-RunValueMissing -Key $runKeyHKCU -Name $runValueNameTray
    Assert-RunValueMissing -Key $runKeyHKLM -Name $runValueNameTray

    if ($wantPurgeProgramData) {
      Wait-PathMissing -Path $programDataDir -TimeoutSeconds 30
    }
  }

  if ($regTouched) {
    # Restore pre-existing autorun settings (if any) to avoid impacting developer machines.
    Remove-RunValueBestEffort -Hive "HKCU" -Name $runValueNameTray
    Remove-RunValueBestEffort -Hive "HKLM" -Name $runValueNameTray
    if ($prevHKCU.Exists) {
      Set-RunValue -Hive "HKCU" -Name $runValueNameTray -Value $prevHKCU.Value
    }
    if ($prevHKLM.Exists) {
      Set-RunValue -Hive "HKLM" -Name $runValueNameTray -Value $prevHKLM.Value
    }
  }
}

Write-Host "MSI service e2e verification passed."
