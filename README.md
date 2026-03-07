# gov-pass

`gov-pass` is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
It is not a proxy, VPN, or general DPI bypass tool. The runtime only collects the
first TLS record, splits that payload, then returns the flow to pass-through mode.

## Support

| Platform | Backend | Status |
| --- | --- | --- |
| Windows 10/11 (x64) | WinDivert | Stable |
| Linux (x86_64) | NFQUEUE | Beta |
| FreeBSD / pfSense | pf divert | Experimental |

## Quick Start

All defaults are tuned for immediate use. In normal cases no flags are required.

### Linux

```bash
sudo ./scripts/install_one_touch.sh
```

Optional TUI controller install:

```bash
sudo INSTALL_TUI=1 ./scripts/install_one_touch.sh
```

Release installer:

```bash
curl -fsSL https://raw.githubusercontent.com/homelabird/gov-pass/main/scripts/install_one_touch_curl.sh | sudo bash
```

### Windows

Run in Administrator PowerShell:

```powershell
.\scripts\install_one_touch.ps1
```

### FreeBSD

```bash
sudo ./scripts/install_one_touch.sh
```

FreeBSD requires manual `pf` anchor setup from [`docs/pf/`](docs/pf/). See
[`docs/DESIGN_BSD.md`](docs/DESIGN_BSD.md) for the divert model.

## Manual Build

Requirements:

- Go 1.21+
- Git

Linux:

```bash
go build -o dist/splitter ./cmd/splitter
sudo ./dist/splitter
```

Windows:

```powershell
go build -o dist\splitter.exe .\cmd\splitter
.\dist\splitter.exe
```

## Runtime Behavior

- Linux installs NFQUEUE rules automatically, disables GRO/GSO/TSO on the
  detected egress interface, and restores offload settings on exit when
  possible.
- Windows auto-installs or auto-downloads WinDivert when required.
- FreeBSD expects `pf divert-to` rules to be managed outside the binary.
- Stop the runtime with `Ctrl+C` or `SIGTERM`.

## TUI Controller

`gov-pass-tui` is a terminal TUI for service control. It supports
start/stop/restart and boot enable/disable on Linux, Windows, and FreeBSD.
On Linux, the TUI also exposes detailed service status and recent journal logs
for faster troubleshooting.

Build:

```bash
make build-tui
```

Install on Linux:

```bash
sudo make install-tui
```

## Configuration

Defaults are intended to work without tuning. When you do need overrides, use
CLI flags or a JSON config file.

Config precedence:

`built-in defaults < config file < explicit CLI flags`

Most useful flags:

- `--config <path>`: load JSON config
- `--check`: run preflight checks and exit
- `--check-json`: preflight checks in JSON
- `--split-mode`: `tls-hello` or `immediate`
- `--split-chunk`: first segment size in bytes
- `--auto-rules` / `--auto-offload` (Linux): disable automatic helpers
- `--service` (Windows): run under the Windows service wrapper

Run `splitter --help` for the full flag list on the current platform.

## Docs

- [`docs/INDEX.md`](docs/INDEX.md): documentation map
- [`SECURITY.md`](SECURITY.md): privilege model and hardening
- [`docs/PACKAGING.md`](docs/PACKAGING.md): package and service layout
- [`docs/DESIGN_COMMON.md`](docs/DESIGN_COMMON.md): shared engine behavior
- [`docs/DESIGN.md`](docs/DESIGN.md): Windows design
- [`docs/DESIGN_LINUX.md`](docs/DESIGN_LINUX.md): Linux design
- [`docs/DESIGN_BSD.md`](docs/DESIGN_BSD.md): FreeBSD / pf design

## Development

```bash
go test ./...
go vet ./...
```

## Security And Notices

- [`SECURITY.md`](SECURITY.md)
- [`docs/THIRD_PARTY_NOTICES.md`](docs/THIRD_PARTY_NOTICES.md)
- [`docs/THIRD_PARTY_SOURCES.md`](docs/THIRD_PARTY_SOURCES.md)
