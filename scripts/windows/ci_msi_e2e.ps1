param(
  [string]$MsiPath = ""
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

try {
  # Best-effort uninstall any previous install matching this package (1605 = not installed).
  Invoke-MsiExec -Args @("/x", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010, 1605) | Out-Null

  # Clean up ProgramData residue from previous runs so log assertions are stable.
  try {
    Stop-Service -Name $svcName -ErrorAction SilentlyContinue
  } catch {
    # ignore
  }
  $cfgPath = "C:\\ProgramData\\gov-pass\\config.json"
  $logPath = "C:\\ProgramData\\gov-pass\\splitter.log"
  try { Remove-Item -Force -ErrorAction SilentlyContinue $cfgPath } catch { }
  try { Remove-Item -Force -ErrorAction SilentlyContinue $logPath } catch { }

  # Install.
  Invoke-MsiExec -Args @("/i", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010) | Out-Null

  $svc = Wait-ServiceRunning -Name $svcName -TimeoutSeconds 60

  $installDir = Join-Path $env:ProgramFiles "gov-pass"
  $exePath = Join-Path $installDir "splitter.exe"
  if (-not (Test-Path $exePath)) {
    throw "splitter.exe not found: $exePath"
  }

  $menuDir = Join-Path $env:ProgramData "Microsoft\\Windows\\Start Menu\\Programs\\gov-pass"
  $lnkStart = Join-Path $menuDir "Start gov-pass service (Admin).lnk"
  $lnkStop = Join-Path $menuDir "Stop gov-pass service (Admin).lnk"
  $lnkReload = Join-Path $menuDir "Reload gov-pass config (Admin).lnk"
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
} finally {
  # Best-effort uninstall.
  try {
    Invoke-MsiExec -Args @("/x", $MsiPath, "/qn", "/norestart") -OkExitCodes @(0, 3010, 1605) | Out-Null
  } catch {
    Write-Host "warning: uninstall failed: $($_.Exception.Message)"
  }
  try {
    Wait-ServiceMissing -Name $svcName -TimeoutSeconds 60
  } catch {
    Write-Host "warning: service removal check failed: $($_.Exception.Message)"
  }
}

Write-Host "MSI service e2e verification passed."
