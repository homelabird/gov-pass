# RPM/DEB Installation and Run Guide

This guide provides a practical flow to build, install, run, and remove `gov-pass` Linux packages.

Targets:
- RPM: Fedora, RHEL, Rocky, AlmaLinux
- DEB: Debian, Ubuntu

## One-touch Install via curl

```bash
curl -fsSL https://raw.githubusercontent.com/homelabird/gov-pass/main/scripts/install_one_touch_curl.sh | bash
```

Install with TUI controller in one step:

```bash
curl -fsSL https://raw.githubusercontent.com/homelabird/gov-pass/main/scripts/install_one_touch_curl.sh | sudo INSTALL_TUI=1 bash
```

Linux TUI mode note:
- `gov-pass-tui` starts a terminal TUI controller (nmtui-like via `whiptail` when available, plain fallback otherwise).

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
