# gov-pass

`gov-pass` is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
It supports lightweight deployment on Windows, Linux, and FreeBSD targets with
platform-native packet interception paths.  
`FreeBSD / pfSense` support is currently experimental and not production.

## Why gov-pass

- Reduce TLS handshake overhead on constrained networks.
- Route outbound TCP/443 traffic through a dedicated flow splitter.
- Keep the binary small and service-friendly for desktop and server use.
- Inspired by the `goodbye-dpi` project, gov-pass is designed for one-touch operation for quick TCP/443 split setup.
- `gov-pass` was created to help in restrictive network environments, not to bypass enterprise security appliances; it focuses on split-only first-ClientHello handling to reduce fragility from coarse SNI filtering, with no roadmap toward enterprise-appliance bypass.

## Project support

| Platform | Backend | Status |
|---|---|---|
| Windows 10/11 (x64) | WinDivert | Stable |
| Linux (x86_64) | NFQUEUE | Beta |
| FreeBSD / pfSense | pf divert | Experimental (not production) |

## Requirements

- Go 1.21+
- Git

## Quick start

All defaults are tuned for immediate use — **no flags are required**.

### Linux

```bash
go build -o dist/splitter ./cmd/splitter
sudo ./dist/splitter
```

### Windows

```powershell
go build -o dist\splitter.exe .\cmd\splitter
.\dist\splitter.exe
```

On Linux the binary will automatically:
- install NFQUEUE rules via `nft` or `iptables`
- disable GRO/GSO/TSO on the detected egress interface
- restore offload settings on exit

On Windows the binary will automatically:
- download and install the WinDivert driver if missing
- uninstall the driver on exit

FreeBSD requires manual pf anchor configuration (see [docs/pf/](docs/pf/)).

Stop with `Ctrl+C` or `SIGTERM`.

## Tray UI (one-touch GUI)

`gov-pass-tray` provides a system-tray GUI for both Windows and Linux that lets
you toggle protection on/off with a single click — similar to how commercial VPN
apps (e.g. Unicorn) work, but with a minimal, distraction-free interface.

### SET
> linux
``` bash
sudo dnf install libayatana-appindicator-gtk3-devel
```

### Build

```bash
# Windows
go build -ldflags -H=windowsgui -o dist\gov-pass-tray.exe .\cmd\gov-pass-tray

# Linux (requires libayatana-appindicator3-dev and libgtk-3-dev)
go build -o dist/gov-pass-tray ./cmd/gov-pass-tray
```

## CLI flag reference

Run `splitter --help` to see all flags with current defaults.
Every flag has a sensible default; override only when needed.

### Common flags (all platforms)

| Flag | Default | Description |
|---|---|---|
| `--split-mode` | `tls-hello` | Split trigger: `tls-hello` or `immediate` |
| `--split-chunk` | `5` | First segment size in bytes |
| `--collect-timeout` | `250ms` | Reassembly collect timeout |
| `--max-buffer` | `65536` | Max reassembly buffer per flow (bytes) |
| `--max-held-pkts` | `32` | Max held packets per flow |
| `--max-seg-payload` | `1460` | Max segment payload size (`0`=unlimited) |
| `--workers` | CPU count | Worker count for sharded processing |
| `--flow-timeout` | `30s` | Idle timeout for flow cleanup |
| `--gc-interval` | `5s` | Flow GC interval |
| `--max-flows-per-worker` | `4096` | Max tracked flows per worker (`0`=unlimited) |
| `--max-reassembly-bytes-per-worker` | `67108864` | Reassembly memory cap per worker (`0`=unlimited) |
| `--max-held-bytes-per-worker` | `67108864` | Held packet memory cap per worker (`0`=unlimited) |
| `--shutdown-fail-open-timeout` | `5s` | Drain timeout during shutdown |
| `--shutdown-fail-open-max-pkts` | `200000` | Max packets reinjected on shutdown |
| `--adapter-flush-timeout` | `2s` | Adapter flush time on shutdown |

### Linux flags

| Flag | Default | Description |
|---|---|---|
| `--queue-num` | `100` | NFQUEUE number |
| `--mark` | `1` | SO_MARK for reinjected packets |
| `--auto-rules` | `true` | Auto install/uninstall nft/iptables rules |
| `--auto-offload` | `true` | Auto disable GRO/GSO/TSO (ethtool) |
| `--auto-offload-restore` | `true` | Restore offload settings on exit |
| `--auto-install-tools` | `true` | Auto install missing system tools via package manager |
| `--iface` | auto-detect | Egress interface for offload control |
| `--no-loopback` | `false` | Include loopback in NFQUEUE rules |
| `--queue-maxlen` | `4096` | NFQUEUE max length (`0`=kernel default) |
| `--copy-range` | `65535` | NFQUEUE copy range in bytes |

### Windows flags

| Flag | Default | Description |
|---|---|---|
| `--filter` | `outbound and ip and tcp.DstPort == 443` | WinDivert filter expression |
| `--queue-len` | `4096` | WinDivert queue length |
| `--queue-time` | `2000` | WinDivert queue time (ms) |
| `--queue-size` | `33554432` | WinDivert queue size (bytes) |
| `--windivert-dir` | exe directory | Directory containing WinDivert files |
| `--auto-install` | `true` | Auto install/start WinDivert driver |
| `--auto-uninstall` | `true` | Auto uninstall driver on exit |
| `--auto-download-windivert` | `true` | Auto download WinDivert if missing |
| `--service` | `false` | Run as Windows service |
| `--config` | _(service only)_ | JSON config file path |

### FreeBSD flags

| Flag | Default | Description |
|---|---|---|
| `--divert-port` | `10000` | pf divert-to port |

Privileges: Linux requires root (or `CAP_NET_ADMIN` + `CAP_NET_RAW`);
Windows requires Administrator.

## Project docs

- Main docs index: [docs/INDEX.md](docs/INDEX.md)
- Security and operations: [SECURITY.md](SECURITY.md)
- Code signing: [docs/CODESIGNING.md](docs/CODESIGNING.md)
- Third-party notices: [docs/THIRD_PARTY_NOTICES.md](docs/THIRD_PARTY_NOTICES.md)
- Third-party source records: [docs/THIRD_PARTY_SOURCES.md](docs/THIRD_PARTY_SOURCES.md)

## Security notes

- Core runtime dependency for Windows: WinDivert 2.2.2-A under [third_party](third_party).
- For threat model and release/security guidance, see [SECURITY.md](SECURITY.md).

## Contributing

- Open PRs after preparing a short issue or design note.
- Keep platform-specific changes in [cmd/](cmd/), [internal/](internal/), [scripts/](scripts/), and [docs/](docs/).

## Open Source and third-party notices

This section matches the notice format used by
[docs/THIRD_PARTY_NOTICES.md](docs/THIRD_PARTY_NOTICES.md).

### Inspired project

- [GoodbyeDPI](https://github.com/ValdikSS/GoodbyeDPI)
  - https://github.com/ValdikSS/GoodbyeDPI/blob/master/LICENSE

### Bundled

- WinDivert 2.2.2-A (binaries/docs)
  - License: [third_party/windivert/WinDivert-2.2.2-A/LICENSE](third_party/windivert/WinDivert-2.2.2-A/LICENSE)

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

### Go module dependencies (tray UI — Windows & Linux)

- github.com/getlantern/systray v1.2.2 (Apache-2.0)
  - https://github.com/getlantern/systray/blob/v1.2.2/LICENSE
- github.com/getlantern/golog v0.0.0-20190830074920-4ef2e798c2d7 (Apache-2.0)
  - https://github.com/getlantern/golog/blob/4ef2e798c2d7/LICENSE
