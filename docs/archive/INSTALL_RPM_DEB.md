# RPM/DEB Installation and Run Guide

This guide provides a practical flow to build, install, run, and remove `gov-pass` Linux packages.

Targets:
- RPM: Fedora, RHEL, Rocky, AlmaLinux
- DEB: Debian, Ubuntu

## One-touch Install via Release Asset

```bash
TAG="vX.Y.Z"
PUBKEY="/path/to/release-signing-pub.pem"
BASE_URL="https://github.com/homelabird/gov-pass/releases/download/${TAG}"

curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/install_one_touch_curl.sh" -o install_one_touch_curl.sh
curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/SHA256SUMS" -o SHA256SUMS
curl --proto '=https' --tlsv1.2 -fL "${BASE_URL}/SHA256SUMS.sig" -o SHA256SUMS.sig
openssl dgst -sha256 -verify "${PUBKEY}" -signature SHA256SUMS.sig SHA256SUMS
awk '$2 == "install_one_touch_curl.sh" { print $1 "  install_one_touch_curl.sh" }' SHA256SUMS | sha256sum -c -
sudo GOV_PASS_VERSION="${TAG}" GOV_PASS_RELEASE_PUBKEY_PATH="${PUBKEY}" bash ./install_one_touch_curl.sh
```

Install with TUI controller in one step:

```bash
sudo INSTALL_TUI=1 GOV_PASS_VERSION="${TAG}" GOV_PASS_RELEASE_PUBKEY_PATH="${PUBKEY}" bash ./install_one_touch_curl.sh
```

Linux TUI mode note:
- `gov-pass-tui` starts the Bubble Tea ON/OFF controller.

Package-manager detection order:
- `apt-get` + `dpkg`
- `dnf`
- `yum`
- `zypper`
- `rpm` (fallback)
- tarball install (last fallback)

Optional environment variables:
- `GOV_PASS_VERSION=vX.Y.Z` to install a specific release tag
- `NO_START=1` to install without starting the systemd service
- `INSTALL_TUI=1` to also install `gov-pass-tui` (Linux/FreeBSD/Windows TUI controller)
- `REPO_OWNER` / `REPO_NAME` to target a fork

## Common Preparation

```bash
cd /home/homelab/Downloads/project/gov-pass
```

Installed paths:
- Binary: `/usr/libexec/gov-pass/splitter`
- Service: `gov-pass`

## RPM Workflow

### Build

```bash
rpmbuild -ba packaging/rpm/gov-pass.spec
ls -1 ~/rpmbuild/RPMS/x86_64/gov-pass-*.x86_64.rpm
```

### Install

```bash
sudo rpm -Uvh ~/rpmbuild/RPMS/x86_64/gov-pass-*.x86_64.rpm
```

### Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gov-pass
```

### Verify

```bash
systemctl status gov-pass --no-pager
journalctl -u gov-pass -n 100 --no-pager
```

### Stop/Restart/Remove

```bash
sudo systemctl stop gov-pass
sudo systemctl restart gov-pass
sudo rpm -e gov-pass
```

RPM config file:
- `/etc/sysconfig/gov-pass`

## DEB Workflow

### Build

```bash
./packaging/deb/build_deb.sh
ls -1 dist/gov-pass_*_amd64.deb
```

### Install

```bash
sudo dpkg -i dist/gov-pass_*_amd64.deb
```

If dependency resolution fails:

```bash
sudo apt-get -f install -y
sudo dpkg -i dist/gov-pass_*_amd64.deb
```

### Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gov-pass
```

### Verify

```bash
systemctl status gov-pass --no-pager
journalctl -u gov-pass -n 100 --no-pager
```

### Stop/Restart/Remove

```bash
sudo systemctl stop gov-pass
sudo systemctl restart gov-pass
sudo dpkg -r gov-pass
```

DEB config file:
- `/etc/default/gov-pass`

## Manual Run (without systemd)

```bash
sudo /usr/libexec/gov-pass/splitter
```

Root is required for the default Linux runtime path.

## Quick Troubleshooting

```bash
systemctl status gov-pass --no-pager
journalctl -u gov-pass -n 200 --no-pager
```

Common causes:
- Missing tools: `nftables`, `iptables`, `ethtool`, `iproute`
- Permission/capability restrictions
- Egress interface detection failure (set `--iface`)
