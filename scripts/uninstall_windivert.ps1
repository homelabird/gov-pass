param(
  [string]$ServiceName = "WinDivert"
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

sc.exe stop $ServiceName | Out-Host
sc.exe delete $ServiceName | Out-Host
