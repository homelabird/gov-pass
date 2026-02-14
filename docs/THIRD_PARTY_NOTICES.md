# Third-Party Notices

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
- golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sys/+/refs/tags/v0.0.0-20220128215802-99c3d69c2c27:LICENSE
