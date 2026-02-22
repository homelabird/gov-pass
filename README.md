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
