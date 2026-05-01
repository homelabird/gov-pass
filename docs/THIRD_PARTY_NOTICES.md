# Third-Party Notices

This project includes and depends on third-party software. Versions are listed
in `go.mod`/`go.sum` for Go modules.
For upstream sources and local patches, see `docs/THIRD_PARTY_SOURCES.md`.

## Bundled

- WinDivert 2.2.2-A (binaries/docs)
  - License: [third_party/windivert/WinDivert-2.2.2-A/LICENSE](third_party/windivert/WinDivert-2.2.2-A/LICENSE)

## Inspired-by project

- GoodbyeDPI
  - https://github.com/ValdikSS/GoodbyeDPI/blob/master/LICENSE

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
- golang.org/x/net v0.51.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/net/+/refs/tags/v0.51.0:LICENSE
- golang.org/x/sync v0.19.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sync/+/refs/tags/v0.19.0:LICENSE
- golang.org/x/sys v0.41.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sys/+/refs/tags/v0.41.0:LICENSE
- golang.org/x/text v0.34.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/text/+/refs/tags/v0.34.0:LICENSE

## Go module dependencies (TUI controller)

- github.com/aymanbagabas/go-osc52/v2 v2.0.1 (MIT)
  - https://github.com/aymanbagabas/go-osc52/blob/v2.0.1/LICENSE
- github.com/charmbracelet/bubbletea v1.3.10 (MIT)
  - https://github.com/charmbracelet/bubbletea/blob/v1.3.10/LICENSE
- github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc (MIT)
  - https://github.com/charmbracelet/colorprofile/blob/f60798e515dc/LICENSE
- github.com/charmbracelet/lipgloss v1.1.0 (MIT)
  - https://github.com/charmbracelet/lipgloss/blob/v1.1.0/LICENSE
- github.com/charmbracelet/x/ansi v0.10.1 (MIT)
  - https://github.com/charmbracelet/x/blob/ansi/v0.10.1/ansi/LICENSE
- github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd (MIT)
  - https://github.com/charmbracelet/x/blob/2c3ea96c31dd/cellbuf/LICENSE
- github.com/charmbracelet/x/term v0.2.1 (MIT)
  - https://github.com/charmbracelet/x/blob/term/v0.2.1/term/LICENSE
- github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f (MIT)
  - https://github.com/erikgeiser/coninput/blob/1c3628e74d0f/LICENSE
- github.com/lucasb-eyer/go-colorful v1.2.0 (MIT)
  - https://github.com/lucasb-eyer/go-colorful/blob/v1.2.0/LICENSE
- github.com/mattn/go-isatty v0.0.20 (MIT)
  - https://github.com/mattn/go-isatty/blob/v0.0.20/LICENSE
- github.com/mattn/go-localereader v0.0.1 (MIT)
  - https://github.com/mattn/go-localereader/blob/v0.0.1/README.md
- github.com/mattn/go-runewidth v0.0.16 (MIT)
  - https://github.com/mattn/go-runewidth/blob/v0.0.16/LICENSE
- github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 (MIT)
  - https://github.com/muesli/ansi/blob/276c6243b2f6/LICENSE
- github.com/muesli/cancelreader v0.2.2 (MIT)
  - https://github.com/muesli/cancelreader/blob/v0.2.2/LICENSE
- github.com/muesli/termenv v0.16.0 (MIT)
  - https://github.com/muesli/termenv/blob/v0.16.0/LICENSE
- github.com/rivo/uniseg v0.4.7 (MIT)
  - https://github.com/rivo/uniseg/blob/v0.4.7/LICENSE.txt
- github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e (MIT)
  - https://github.com/xo/terminfo/blob/abceb7e1c41e/LICENSE
- golang.org/x/sys v0.41.0 (BSD-3-Clause)
  - https://cs.opensource.google/go/x/sys/+/refs/tags/v0.41.0:LICENSE
