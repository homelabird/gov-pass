# Maintainers

This document covers release, packaging, and signing work. Operator-facing run
instructions stay in [`../README.md`](../README.md).

## Release Surface

Tag releases are built by [`../.github/workflows/release.yml`](../.github/workflows/release.yml).
The release bundle publishes:

- source and platform archives
- Windows MSI and zip artifacts
- Linux `.deb` and `.rpm`
- signed checksum manifests
- `install_one_touch_curl.sh`

Signed manifest helpers live in:

- [`../scripts/ci/sign_release_manifests.sh`](../scripts/ci/sign_release_manifests.sh)
- [`../scripts/ci/verify_release_manifest_signature.sh`](../scripts/ci/verify_release_manifest_signature.sh)

## Windows

Windows release artifacts include:

- `splitter.exe`
- `gov-pass-tui.exe`
- `gov-pass-msi-helper.exe`
- `WinDivert.dll`
- `WinDivert64.sys` or `WinDivert.sys`
- `gov-pass-<tag>-windows-amd64.msi`

MSI packaging is driven from [`../installer/windows/`](../installer/windows/) and
the WiX template [`../installer/windows/gov-pass.wxs.in`](../installer/windows/gov-pass.wxs.in).

Code signing is optional in CI but required for public Windows releases.
Configure:

- `WINDOWS_CODESIGN_PFX_B64`
- `WINDOWS_CODESIGN_PFX_PASSWORD`
- `WINDOWS_CODESIGN_TIMESTAMP_URL` or `WINDOWS_CODESIGN_TIMESTAMP_URLS`

Signing helper:

- [`../scripts/ci/sign_windows_artifacts.sh`](../scripts/ci/sign_windows_artifacts.sh)

Smoke verification:

- [`../scripts/windows/ci_msi_e2e.ps1`](../scripts/windows/ci_msi_e2e.ps1)

PowerShell signature check:

```powershell
Get-AuthenticodeSignature .\dist\release\gov-pass-*-windows-amd64.msi |
  Format-List Status,StatusMessage,SignerCertificate,TimeStamperCertificate
```

## Linux

Package sources live under:

- [`../packaging/deb/`](../packaging/deb/)
- [`../packaging/rpm/`](../packaging/rpm/)

Release installer paths:

- [`../scripts/install_one_touch.sh`](../scripts/install_one_touch.sh)
- [`../scripts/install_one_touch_curl.sh`](../scripts/install_one_touch_curl.sh)

The release workflow signs checksum manifests for release assets. Public
bootstrap flows should be verified against the signed manifest before execution.

## FreeBSD

FreeBSD packaging is source-installer oriented. The installer lays down:

- `splitter`
- optional `gov-pass-tui`
- `/usr/local/etc/rc.d/gov-pass`
- `/usr/local/libexec/gov-pass/` helper scripts
- `/usr/local/etc/gov-pass/pf.anchor.conf`

Smoke verification:

- [`../scripts/freebsd/ci_tui_smoke.sh`](../scripts/freebsd/ci_tui_smoke.sh)

## Validation

Use the release workflow for canonical artifacts. Local maintainer checks
should at least cover:

- `go test ./...`
- Windows MSI smoke when packaging changes
- signed manifest verification for release assets
