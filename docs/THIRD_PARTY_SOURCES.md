# Third-Party Sources

This file records vendored upstream sources in `third_party/` and the local
patches applied on top of those sources.

## Policy

- Vendor upstream sources under `third_party/<name>`.
- Record upstream URL and version/tag/commit here.
- Store local changes as patch files under `third_party/patches/<name>/`.
- Keep patches minimal and explain the rationale.

## Components

### WinDivert

- Upstream: https://github.com/basil00/WinDivert
- Version: 2.2.2-A
- Local path: `third_party/windivert/WinDivert-2.2.2-A`
- Bundled archive: `third_party/WinDivert.zip`
- Local patches: none (binaries/docs are bundled as-is)

### go-nfqueue

- Upstream: https://github.com/florianl/go-nfqueue
- Version: v1.3.2
- Local path: `third_party/go-nfqueue`
- Local patches:
  - `third_party/patches/go-nfqueue/0001-android-build-tags.patch` (add android build tags)

### netlink

- Upstream: https://github.com/mdlayher/netlink
- Version: v1.6.0
- Local path: `third_party/netlink`
- Local patches:
  - `third_party/patches/netlink/0001-android-build-tags.patch` (add android build tags)

## Update flow (manual)

1. Check out the upstream version to a temporary directory.
2. Replace `third_party/<name>` with the upstream contents.
3. Apply patches from `third_party/patches/<name>/`.
4. Update this file with the new version and patch list if needed.
