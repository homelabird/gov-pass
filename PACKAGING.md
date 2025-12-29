# Packaging and Driver Setup

This project requires WinDivert user-mode DLL and kernel driver files.
Bundle these files with the executable for a self-contained distribution.

Default deployment layout:
- `dist\splitter.exe`
- `dist\WinDivert.dll`
- `dist\WinDivert64.sys` (or `WinDivert.sys`)
- `dist\WinDivert.cat`

The runtime auto-install uses the executable directory by default, so placing
the WinDivert files next to the exe is the standard setup. The service name
is fixed as `WinDivert`.

## Build

```powershell
go build -o dist\splitter.exe .\cmd\splitter
```

## Package (copy DLL/SYS/CAT)

```powershell
.\scripts\package.ps1 -ExePath dist\splitter.exe -WinDivertDir C:\path\to\WinDivert
```

This copies:
- WinDivert.dll
- WinDivert64.sys (or WinDivert.sys if present)
- WinDivert.cat (if present)

## Install/Uninstall (optional)

```powershell
.\scripts\install_windivert.ps1 -WinDivertDir dist
.\scripts\uninstall_windivert.ps1
```

## Runtime auto-install

By default the app will auto-install/start the driver if needed and
uninstall it on exit if it created the service.

Flags:
- --windivert-dir: directory containing WinDivert.dll/.sys/.cat
- --windivert-sys: override driver sys filename
- --auto-install: enable auto install/start (default true)
- --auto-uninstall: uninstall if installed by this run (default true)
