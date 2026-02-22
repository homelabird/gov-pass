# gov-pass

`gov-pass` is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
It can run as a CLI process or as a Windows service (installer package).

## What this project supports

- Windows 10/11 x64 (WinDivert) ??main support
- Linux x86_64 (NFQUEUE) ??beta
- FreeBSD / pfSense (pf divert) ??experimental

## Requirements

- Go 1.21+
- Git

## Quick start

### Windows (CLI)

```powershell
go build -o dist\splitter.exe .\cmd\splitter
.\dist\splitter.exe
```

### Linux

```bash
go build -o dist/splitter ./cmd/splitter
sudo ./dist/splitter --queue-num 100 --mark 1
```

## Common usage notes

- Most defaults are safe for local testing; production environments should set
  `workers`, DoS guard rails, and timeouts explicitly.
- The CLI supports CLI flags for:
  - `--max-seg-payload`
  - `--max-flows-per-worker`
  - `--max-reassembly-bytes-per-worker`
  - `--max-held-bytes-per-worker`
- Run with admin/root permissions where kernel hooks are required.

## Project docs

- Main docs index: `docs/INDEX.md`
- Security & operations: `SECURITY.md`
- Code signing: `docs/CODESIGNING.md`
- License and third-party notices: `docs/THIRD_PARTY_NOTICES.md`

## Security

- Third-party binary/runtime dependency used: WinDivert 2.2.2-A (Windows path in `third_party`)
- See `SECURITY.md` and `docs/THIRD_PARTY_NOTICES.md` for details.

## Contributing

- Open PRs against the repository after creating a concise issue or design note.
- Keep platform-specific changes isolated in `cmd`, `scripts`, and `docs`.

## Open Source Used

This project includes and depends on third-party software. Versions are listed
in `go.mod`/`go.sum` for Go modules.
For upstream sources and local patches, see `docs/THIRD_PARTY_SOURCES.md`.

## Bundled

- WinDivert 2.2.2-A (binaries/docs)
  - License: `third_party\windivert\WinDivert-2.2.2-A\LICENSE`

## System dependencies (Linux)

None. The Linux NFQUEUE path uses a pure-Go netlink client (`go-nfqueue`).

## Go module dependencies (Linux NFQUEUE path)

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

## Go module dependencies (Windows tray UI)

- github.com/getlantern/systray v1.2.2 (Apache-2.0)
  - https://github.com/getlantern/systray/blob/v1.2.2/LICENSE
- github.com/getlantern/golog v0.0.0-20190830074920-4ef2e798c2d7 (Apache-2.0)
  - https://github.com/getlantern/golog/blob/4ef2e798c2d7/LICENSE