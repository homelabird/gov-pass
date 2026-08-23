# gov-pass

`gov-pass` is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
It is not a proxy, VPN, or general DPI bypass tool. The runtime only collects the
first TLS record, splits that payload, then returns the flow to pass-through mode.

## Support

| Platform | Backend | Status |
| --- | --- | --- |
| Windows 10/11 (x64) | WinDivert | Stable |
| Linux (x86_64) | NFQUEUE | Stable |
| FreeBSD / pfSense | pf divert | Experimental |

Linux on supported x86_64 hosts is suitable for day-to-day use. For
multi-egress, VPN, or container environments, pin `--iface` explicitly in the
service defaults before enabling the service.

## Quick Start

All defaults are tuned for immediate use. In normal cases no flags are required.

### Linux

```bash
sudo ./scripts/install_one_touch.sh
```

This installs and starts the service and adds `gov-pass-tui` to `PATH`. Run the
controller from any terminal:

```bash
gov-pass-tui
```

Service installs keep `--auto-install-tools=false --auto-offload=false` in
`/etc/default/gov-pass` so normal operation does not change NIC-wide offload.
On multi-egress, VPN, or container hosts, set `--iface` explicitly there before enabling the service.

Runtime-only install without the TUI controller:

```bash
sudo INSTALL_TUI=0 ./scripts/install_one_touch.sh
```

Source installs resolve `go` only from trusted system paths. If Go is installed
elsewhere, set `GOV_PASS_GO_BIN` to an absolute, non-symlinked `go` binary path:

```bash
sudo GOV_PASS_GO_BIN="$(command -v go)" ./scripts/install_one_touch.sh
```

Release installer:

```bash
TAG="vX.Y.Z"
PUBKEY="/path/to/release-signing-pub.pem"
BASE_URL="https://github.com/homelabird/gov-pass/releases/download/${TAG}"

curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/install_one_touch_curl.sh" -o install_one_touch_curl.sh
curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/SHA256SUMS" -o SHA256SUMS
curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/SHA256SUMS.sig" -o SHA256SUMS.sig
openssl dgst -sha256 -verify "${PUBKEY}" -signature SHA256SUMS.sig SHA256SUMS
awk '$2 == "install_one_touch_curl.sh" { print $1 "  install_one_touch_curl.sh" }' SHA256SUMS | sha256sum -c -
sudo GOV_PASS_VERSION="${TAG}" \
  GOV_PASS_RELEASE_PUBKEY_PATH="${PUBKEY}" \
  bash ./install_one_touch_curl.sh
```

Verify the installer script against the signed release manifest before running it
with `sudo`. The installer then verifies detached signatures on published
SHA256 manifests before installing any release asset. Obtain the release
signing public key from a maintainer-controlled channel before running it.

### Windows

For a published release, download the signed `gov-pass-*-windows-amd64.msi`
from [GitHub Releases](https://github.com/homelabird/gov-pass/releases), open it,
then launch **gov-pass TUI** from the Start menu. Windows requests Administrator
permission when the controller opens so its service actions work.

For a source build, run in Administrator PowerShell:

```powershell
.\scripts\install_one_touch.ps1
```

If Go is not installed under the standard Program Files location, set
`$env:GOV_PASS_GO_BIN` to the absolute `go.exe` path before running the helper.

The source helper builds both binaries, installs them under `C:\Program Files\gov-pass`,
creates and starts the `gov-pass` Windows service, creates the default
`C:\ProgramData\gov-pass\config.json`, and adds **gov-pass TUI** to the Start menu.
Use `-SkipDriverInstall` only when you want build artifacts in `dist` without
installing either service.

By default, `splitter.exe` refuses to reconfigure an existing `WinDivert`
service when it already points at another valid driver path. Use
`--allow-service-takeover` or `windivert.allow_service_takeover=true` only for
recovery or controlled migrations; cleanup restores the previous service path.

MSI/service layout:

- binaries: `C:\Program Files\gov-pass\`
- service config: `C:\ProgramData\gov-pass\config.json`
- service log: `C:\ProgramData\gov-pass\splitter.log`

Reload config in place:

```powershell
sc.exe control gov-pass paramchange
```

Use [`docs/examples/splitter.windows.json`](docs/examples/splitter.windows.json)
as the service config template, [`docs/schema/`](docs/schema/) for the versioned
JSON field contract, and `splitter.exe --print-reloadability` to inspect
restart-required settings on the current build.

### FreeBSD

```bash
sudo ./scripts/install_one_touch.sh
```

Then edit `/usr/local/etc/gov-pass/pf.anchor.conf`, apply it, and enable the
service:

```bash
sudo /usr/local/libexec/gov-pass/install_pf_anchor.sh
sudo sysrc gov_pass_enable=YES
sudo service gov-pass start
```

Use [`docs/pf/`](docs/pf/) for anchor examples and [`docs/DESIGN.md`](docs/DESIGN.md)
for the current FreeBSD caveats. Current scope: reload is restart-only, `pf`
policy remains operator-managed, and the current divert socket path should be
treated as IPv4-only.

`splitter --check` also checks whether `pf` is enabled and whether the
`gov-pass` live anchor has rules loaded. It does not prove that interface
selectors in the anchor match a given pfSense or FreeBSD topology.

## Manual Build

Requirements:

- Go 1.25.8+
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

- Linux installs IPv4 and IPv6 TCP/443 NFQUEUE rules automatically. NIC-wide
  GRO/GSO/TSO changes are opt-in with `--auto-offload=true`; when restore is
  enabled, startup refuses to change offload unless it first captures a restore
  snapshot.
- The packaged/systemd Linux service keeps
  `--auto-install-tools=false --auto-offload=false`; pre-provision `nftables`
  or `iptables`/`ip6tables`. Set `--iface` explicitly before opting into
  offload changes on multi-egress, container, or VPN hosts.
- Windows auto-installs or auto-downloads WinDivert when required.
- FreeBSD remains experimental but now installs an `rc.d` service and `pf`
  helper scripts under `/usr/local/libexec/gov-pass/`; `pf divert-to` policy
  is still operator-managed.
- Stop the runtime with `Ctrl+C` or `SIGTERM`.

## TUI Controller

`gov-pass-tui` is a terminal TUI for service control. It supports
one-button ON/OFF control on Linux, Windows, and FreeBSD. Press Enter or Space,
or click the `[ TURN ON ]` / `[ TURN OFF ]` button.

The panel only shows the current ON/OFF state, one toggle button, and an error
when the service cannot be controlled. Advanced automation remains available
through `--action` without adding more buttons to the screen.

Build:

```bash
make build-tui
```

Install on Linux:

```bash
sudo make install-tui
```

Run against a different service:

```bash
gov-pass-tui --service-name other-service
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
- `--stats-interval`: periodic engine stats log interval (default `0`; opt-in)
- `--auto-rules` (Linux): toggle automatic NFQUEUE rules
- `--auto-offload=true` (Linux): opt in to NIC-wide GRO/GSO/TSO changes
- `--iface` (Linux): pin the egress interface for offload changes
- `--recv-buffer` (Linux): internal NFQUEUE recv buffer (`0` derives from `--queue-maxlen`)
- `--service` (Windows): run under the Windows service wrapper

Run `splitter --help` for the full flag list on the current platform.
Use [`docs/examples/`](docs/examples/) for platform templates and
[`docs/schema/`](docs/schema/) for the JSON schema files. CI validates that each
example stays within its matching schema.

## Docs

- [`SECURITY.md`](SECURITY.md): privilege model and hardening
- [`CONTRIBUTING.md`](CONTRIBUTING.md): local build, test, and CI workflow
- [`docs/DESIGN.md`](docs/DESIGN.md): runtime architecture and platform caveats
- [`docs/schema/`](docs/schema/): JSON config schema documents
- [`docs/MAINTAINERS.md`](docs/MAINTAINERS.md): release, packaging, and signing notes

## Development

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/engine ./cmd/splitter ./cmd/gov-pass-tui
go vet ./...
staticcheck ./...
golangci-lint run ./...
```

## Security And Notices

- [`SECURITY.md`](SECURITY.md)
- [`docs/THIRD_PARTY_NOTICES.md`](docs/THIRD_PARTY_NOTICES.md)
- [`docs/THIRD_PARTY_SOURCES.md`](docs/THIRD_PARTY_SOURCES.md)
