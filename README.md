# gov-pass

Split-only TLS ClientHello splitter for outbound IPv4 TCP 443.

Demo video: https://www.youtube.com/watch?v=is96qPruy40

Docs entry point: `docs/INDEX.md`
Security/ops notes: `SECURITY.md`

Supported platforms:
- Windows 10/11 x64 (WinDivert) - primary
- Linux x86_64 (NFQUEUE) - beta
- FreeBSD / pfSense (pf divert) - experimental

## Windows build (x64)

Prerequisites:
- Windows 10/11 x64
- Go 1.21+
- WinDivert 2.2.2-A x64 files (this repo includes them under
  `third_party\windivert\WinDivert-2.2.2-A\x64`)

Build the binary:
```powershell
go build -o dist\splitter.exe .\cmd\splitter
```

Package WinDivert alongside the binary:
```powershell
.\scripts\package.ps1 `
  -ExePath .\dist\splitter.exe `
  -WinDivertDir .\third_party\windivert\WinDivert-2.2.2-A\x64 `
  -OutDir .\dist
```

Expected files in `dist\`:
- `splitter.exe`
- `WinDivert.dll`
- `WinDivert64.sys`
- `WinDivert.cat` (optional; not included in the official zip)

Optional: install the driver once (requires Admin):
```powershell
.\scripts\install_windivert.ps1 -WinDivertDir .\dist
```

Driver install notes:
- The installer resolves the absolute `.sys` path and will update an existing
  service if its `binPath` points to a missing file.
- To force-update the `binPath` even if it exists, use:
```powershell
.\scripts\install_windivert.ps1 -WinDivertDir .\dist -ForceBinPath
```

Run (Admin PowerShell):
```powershell
.\dist\splitter.exe
```

If `WinDivert.dll` / `WinDivert64.sys` are missing, the app can auto-download the
pinned WinDivert zip (SHA256-verified) and extract the x64 files. You can
disable this with:
```powershell
.\dist\splitter.exe --auto-download-windivert=false
```

## Windows MSI install (x64)

If you have a release build, you can install via the `.msi` package.
The installer includes `splitter.exe` and the WinDivert `dll/sys` files.

After installing, a Windows service named `gov-pass` is installed and started
automatically (Startup type: Automatic).

Notes:
- WinDivert requires Administrator privileges.
- Uninstalling gov-pass does not remove the global WinDivert driver service.
- Service logs are written to `C:\ProgramData\gov-pass\splitter.log`.
- Service config is read from `C:\ProgramData\gov-pass\config.json` (created on first run if missing).
- In service mode, `C:\ProgramData\gov-pass\` is ACL-hardened (SYSTEM/Admin full, Users read-only).
  Editing `config.json` requires Admin.

To manage the service (Admin PowerShell):
```powershell
sc.exe query gov-pass
sc.exe stop gov-pass
sc.exe start gov-pass
sc.exe control gov-pass paramchange  # reload config.json (in-place apply; restart required for some settings)
```

Reload semantics (service):
- Applies in-place: `engine.*` (except worker topology), and non-zero WinDivert `queue_*` values.
- Requires service restart: `windivert.filter`, `windivert_dir` / `windivert_sys`, and reverting `queue_*` to `0` (driver defaults).

Service config example (`C:\ProgramData\gov-pass\config.json`):
```json
{
  "engine": {
    "split_mode": "tls-hello",
    "split_chunk": 5,
    "collect_timeout": "250ms",
    "max_buffer_bytes": 65536,
    "max_held_packets": 32,
    "max_segment_payload": 1460,
    "workers": 8,
    "flow_idle_timeout": "30s",
    "gc_interval": "5s",
    "max_flows_per_worker": 4096,
    "max_reassembly_bytes_per_worker": 67108864,
    "max_held_bytes_per_worker": 67108864,
    "shutdown_fail_open_timeout": "5s",
    "shutdown_fail_open_max_packets": 200000,
    "adapter_flush_timeout": "2s"
  },
  "windivert": {
    "filter": "outbound and ip and tcp.DstPort == 443",
    "queue_len": 4096,
    "queue_time_ms": 2000,
    "queue_size_bytes": 33554432,
    "auto_install_driver": true,
    "auto_download_files": true
  }
}
```

Optional interactive run:
- Use the Start Menu shortcuts:
  - `gov-pass splitter (Admin)`
  - `Start gov-pass service (Admin)`
  - `Stop gov-pass service (Admin)`
  - `Restart gov-pass service (Admin)`
  - `Reload gov-pass config (Admin)`
- Stop the `gov-pass` service first to avoid running two instances.

If you want to keep the driver installed after exit:
```powershell
.\dist\splitter.exe --auto-uninstall=false
```

Optional: cap injected segment payload size (default 1460):
```powershell
.\dist\splitter.exe --max-seg-payload 1200
```

Optional: DoS guards (per worker; fail-open on pressure):
```powershell
.\dist\splitter.exe `
  --max-flows-per-worker 4096 `
  --max-reassembly-bytes-per-worker 67108864 `
  --max-held-bytes-per-worker 67108864
```

Optional: shutdown guardrails (advanced):
```powershell
.\dist\splitter.exe `
  --shutdown-fail-open-timeout 5s `
  --shutdown-fail-open-max-pkts 200000 `
  --adapter-flush-timeout 2s
```

## Linux build and run (NFQUEUE MVP)

Linux support is beta. Expect breaking changes and validate in your environment.

Prerequisites:
- Linux x86_64
- Go 1.21+
- root privileges (default run installs rules and disables offload)

Build:
```bash
go build -o dist/splitter ./cmd/splitter
```

Run (root or with capabilities):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1
```

By default the Linux binary will:
- install NFQUEUE rules using nft or iptables
- disable GRO/GSO/TSO on the egress interface (auto-detected)
- by default, restore offload settings on exit when possible (`--auto-offload-restore=true`)
- auto-install missing system tools (nft/iptables/ip/ethtool) when required

Override defaults:
- `--auto-rules=false` to manage rules manually
- `--auto-offload=false` to skip offload changes
- `--auto-offload-restore=false` to keep offload changes persistent after exit
- `--iface <iface>` to override the auto-detected interface
- `--auto-install-tools=false` to disable package-manager auto install of missing tools

Manual rule install (optional):
```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
```

Optional: cap injected segment payload size (default 1460):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1 --max-seg-payload 1200
```

Optional: DoS guards (per worker; fail-open on pressure):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1 \
  --max-flows-per-worker 4096 \
  --max-reassembly-bytes-per-worker 67108864 \
  --max-held-bytes-per-worker 67108864
```

Optional: shutdown guardrails (advanced):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1 \
  --shutdown-fail-open-timeout 5s \
  --shutdown-fail-open-max-pkts 200000 \
  --adapter-flush-timeout 2s
```

Optional capabilities instead of root:
```bash
sudo setcap 'cap_net_admin,cap_net_raw=+ep' ./dist/splitter
```

If using capabilities, disable auto rules/offload and manage them manually:
```bash
./dist/splitter --auto-rules=false --auto-offload=false --queue-num 100 --mark 1
```

Cleanup rules:
```bash
sudo ./scripts/linux/uninstall_nfqueue.sh --queue-num 100 --mark 1
```

PCAP verification (reinjection/splitting):
```bash
sudo ./scripts/linux/pcap_verify.sh --iface <iface> --cmd "curl -sk https://example.com >/dev/null"
```

Netns integration test (isolated, requires root):
```bash
sudo ./scripts/linux/netns_integration_test.sh --queue-num 100 --mark 1
```

Load probe (basic throughput/latency sanity check):
```bash
./scripts/linux/load_probe.sh --target https://example.com --concurrency 50 --requests 500
```

## FreeBSD / pfSense (pf divert, experimental)

Prerequisites:
- FreeBSD 14.x or pfSense 2.7.x (IPDIVERT enabled)
- root privileges
- pf divert rules in place (see `docs/POC_BSD.md` and `docs/pf/*`)

Build:
```bash
GOOS=freebsd GOARCH=amd64 go build -o dist/splitter-freebsd ./cmd/splitter
```

Run:
```bash
sudo ./dist/splitter-freebsd --divert-port 10000
```

The `--divert-port` must match the pf `divert-to` port.
Use `--max-seg-payload` to cap injected segment size (default 1460).

## Android (root, Magisk)

Android support targets rooted arm64 devices with a custom kernel that enables
NFQUEUE and iptables. Note: Android is currently design/PoC documentation only;
the repo does not ship a maintained `GOOS=android` build target yet. See
`docs/DESIGN_ANDROID.md` and `scripts/android/magisk` for notes.

## Third-party sources included

- WinDivert 2.2.2-A (binaries/docs) in `third_party\windivert\WinDivert-2.2.2-A`
  and `third_party\WinDivert.zip` (license: `third_party\windivert\WinDivert-2.2.2-A\LICENSE`)

See `docs/THIRD_PARTY_NOTICES.md` for additional dependency licenses.
