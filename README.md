# gov-pass

`gov-pass` is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
It supports lightweight deployment on Windows, Linux, and FreeBSD targets with
platform-native packet interception paths.  
`FreeBSD / pfSense` support is currently incomplete (implementation in progress), so it is marked as WIP.

## Why gov-pass

- Reduce TLS handshake overhead on constrained networks.
- Route outbound TCP/443 traffic through a dedicated flow splitter.
- Keep the binary small and service-friendly for desktop and server use.

## Project support

| Platform | Backend | Status |
|---|---|---|
| Windows 10/11 (x64) | WinDivert | Stable |
| Linux (x86_64) | NFQUEUE | Beta |
| FreeBSD / pfSense | pf divert | Incomplete / WIP |

## Requirements

- Go 1.21+
- Git

## Quick start

### 1) Build

```bash
go build -o dist/splitter ./cmd/splitter
```

### 2) Run (CLI)

```bash
sudo ./dist/splitter --queue-num 100 --mark 1
```

### 3) Windows run (CLI)

```powershell
go build -o dist\splitter.exe .\cmd\splitter
.\dist\splitter.exe
```

## Common configuration

The most frequently used flags are shown below:

| Flag | Purpose | Tip |
|---|---|---|
| `--max-seg-payload` | Max TLS payload segment size | Tune with larger payload environments |
| `--max-flows-per-worker` | Flow concurrency limit per worker | Reduce in memory-constrained hosts |
| `--max-reassembly-bytes-per-worker` | Per-worker reassembly memory cap | Increase for high-latency links |
| `--max-held-bytes-per-worker` | Additional buffered bytes per worker | Balance throughput vs memory |

When using kernel hooks, run with required privileges.

- Windows: admin privileges are required for the driver path.
- Linux: root or equivalent capability is required for NFQUEUE.

## Project docs

- Main docs index: `docs/INDEX.md`
- Security and operations: `SECURITY.md`
- Code signing: `docs/CODESIGNING.md`
- Third-party notices: `docs/THIRD_PARTY_NOTICES.md`
- Third-party source records: `docs/THIRD_PARTY_SOURCES.md`

## Security notes

- Core runtime dependency for Windows: WinDivert 2.2.2-A under `third_party`.
- For threat model and release/security guidance, see `SECURITY.md`.

## Contributing

- Open PRs after preparing a short issue or design note.
- Keep platform-specific changes in `cmd/`, `internal/`, `scripts/`, and `docs/`.

## Open Source and third-party notices

This section matches the notice format used by
`docs/THIRD_PARTY_NOTICES.md`.

### Bundled

- WinDivert 2.2.2-A (binaries/docs)
  - License: `third_party\windivert\WinDivert-2.2.2-A\LICENSE`

### System dependencies (Linux)

None. The Linux NFQUEUE path uses a pure-Go netlink client (`go-nfqueue`).

### Go module dependencies (Linux NFQUEUE path)

- github.com/florianl/go-nfqueue v1.3.2 (MIT)
  - https://github.com/florianl/go-nfqueue/blob/v1.3.2/LICENSE
- github.com/mdlayher/netlink v1.6.0 (MIT)
  - https://github.com/mdlayher/netlink/blob/v1.6.0/LICENSE.md
- github.com/mdlayher/socket v0.1.1 (MIT)
  - https://github.com/mdlayher/socket/blob/v0.1.1/LICENSE.md
- github.com/josharian/native v1.0.0 (MIT)
  - https://github.com/josharian/native/blob/v1.0.0/license
- github.com/google/go-cmp v0.5.7 (BSD-3-Clause)
  - https://github.com/google/go-cmp/blob/v0.5.7/LICENSE
- golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd (BSD-3-Clause)
  - https://cs.opensource.google/go/x/net/+/refs/tags/v0.0.0-20220127200216-cd36cc0744dd:LICENSE
- golang.org/x/sync v0.0.0-20210220032951-036812b2e83c (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sync/+/refs/tags/v0.0.0-20210220032951-036812b2e83c:LICENSE
- golang.org/x/sys v0.1.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sys/+/refs/tags/v0.1.0:LICENSE

### Go module dependencies (Windows tray UI)

- github.com/getlantern/systray v1.2.2 (Apache-2.0)
  - https://github.com/getlantern/systray/blob/v1.2.2/LICENSE
- github.com/getlantern/golog v0.0.0-20190830074920-4ef2e798c2d7 (Apache-2.0)
  - https://github.com/getlantern/golog/blob/4ef2e798c2d7/LICENSE
