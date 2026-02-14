# Contributing

## Prereqs

- Go 1.21+
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

### Windows (service)

- Default service config: `C:\ProgramData\gov-pass\config.json`
- Default service log: `C:\ProgramData\gov-pass\splitter.log`
- Reload: `sc.exe control gov-pass paramchange`
- Service mode hardens ACL on `C:\ProgramData\gov-pass\` (SYSTEM/Admin full, Users read-only). Editing `config.json` requires Admin.

### Linux (NFQUEUE)

Build:

```bash
go build -o dist/splitter ./cmd/splitter
```

Default run (root, installs rules + disables offload):

```bash
sudo ./dist/splitter --queue-num 100 --mark 1
```

Manual rule install/remove:

```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
sudo ./scripts/linux/uninstall_nfqueue.sh --queue-num 100 --mark 1
```

## CI / Release

- GitLab CI builds release artifacts on tags (`build_release`).
- Windows MSI E2E verification requires a Windows runner with Administrator privileges:
  - Enable by setting `WINDOWS_E2E=1` for tagged pipelines.
  - Job: `verify_windows_msi_e2e` runs `scripts/windows/ci_msi_e2e.ps1`.

## Engineering Guidelines

- Prefer fail-open over fail-closed on decode/reassembly/timeout/pressure paths.
- Keep shutdown paths bounded (timeouts + max packet caps) so Stop does not hang under load.
- Avoid unbounded growth: flows/held/reassembly bytes should be capped (per worker).
- Keep cross-platform builds working: use build tags and avoid OS-specific code in shared packages without guards.

