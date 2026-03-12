# Contributing

## Prereqs

- Go 1.25.8+
- Windows development/testing may require Administrator privileges (WinDivert driver + service control).
- Linux development/testing may require root or capabilities (`CAP_NET_ADMIN`, `CAP_NET_RAW`) depending on how you run it.

## Quick Checks

```bash
go test ./...
go vet ./...
```

## Local Build/Run

### Windows (interactive)

```powershell
go build -o dist\\splitter.exe .\\cmd\\splitter
```

Run (Admin):

```powershell
.\\dist\\splitter.exe
```

### Windows (TUI controller)

```powershell
go build -o dist\\gov-pass-tui.exe .\\cmd\\gov-pass-tui
```

`gov-pass-tui.exe` is a terminal TUI controller on Windows.

### Windows (service)

- Default service config: `C:\ProgramData\gov-pass\config.json`
- Default service log: `C:\ProgramData\gov-pass\splitter.log`
- Reload: `sc.exe control gov-pass paramchange`
- Service mode hardens ACL on `C:\ProgramData\gov-pass\` (SYSTEM/Admin full, Users read-only). Editing `config.json` requires Admin.

### Windows (MSI)

For MSI packaging details (CI, WiX, and signing), see `docs/MAINTAINERS.md`.

### Linux (NFQUEUE)

Build:

```bash
go build -o dist/splitter ./cmd/splitter
```

Default run (root, installs rules + disables offload):

```bash
sudo ./dist/splitter
```

Manual rule install/remove:

```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
sudo ./scripts/linux/uninstall_nfqueue.sh --queue-num 100 --mark 1
```

## CI / Release

- GitLab CI builds release artifacts on tags (`build_release`).
- GitHub Actions publishes GitHub release assets on tag pushes via [`release.yml`](.github/workflows/release.yml), including `install_one_touch_curl.sh`, signed checksum manifests, and platform bundles.
- Branch pipelines now run:
  - `verify_go_tests`: `go test ./...` + `go vet ./...`
  - `verify_cross_builds`: Linux/FreeBSD/Windows cross-build smoke
  - `verify_release_signing_helpers`: detached-signature helper round-trip
  - `verify_tui_tests`: TUI-focused tests + Windows TUI compile smoke
- GitHub release publishing requires checksum-signing secrets:
  - `RELEASE_CHECKSUM_SIGNING_KEY_PEM` or `RELEASE_CHECKSUM_SIGNING_KEY_PEM_B64`
  - optional `RELEASE_CHECKSUM_SIGNING_KEY_PASS`
- Windows Authenticode signing in GitHub release publishing is optional and uses:
  - `WINDOWS_CODESIGN_PFX_B64`
  - `WINDOWS_CODESIGN_PFX_PASSWORD`
- Linux root/network namespace E2E uses `verify_linux_netns_e2e`:
  - Tag pipelines always run `verify_linux_netns_e2e` and require a `linux-root` runner.
  - Branch/MR pipelines can opt in by setting `LINUX_NETNS_E2E=1`
  - Runner tag requirement: `linux-root`
  - Runner must permit `ip netns` creation and veth pair setup (`CAP_NET_ADMIN`)
  - Job: `verify_linux_netns_e2e`
- Windows MSI E2E verification requires a Windows runner with Administrator privileges:
  - Enable by setting `WINDOWS_E2E=1` for tagged pipelines.
  - Job: `verify_windows_msi_e2e` runs `scripts/windows/ci_msi_e2e.ps1`.
  - Coverage: MSI install/uninstall, service reload/start/stop, and `gov-pass-tui.exe` status/reload/interactive smoke.
- FreeBSD TUI smoke verification is available as an opt-in verify job:
  - Set `FREEBSD_E2E=1`
  - Runner tag requirement: `freebsd-root`
  - Job: `verify_freebsd_tui_smoke` installs via `scripts/install_one_touch.sh` with `INSTALL_TUI=1` and runs `scripts/freebsd/ci_tui_smoke.sh`.

## Engineering Guidelines

- Prefer fail-open over fail-closed on decode/reassembly/timeout/pressure paths.
- Keep shutdown paths bounded (timeouts + max packet caps) so Stop does not hang under load.
- Avoid unbounded growth: flows/held/reassembly bytes should be capped (per worker).
- Keep cross-platform builds working: use build tags and avoid OS-specific code in shared packages without guards.
