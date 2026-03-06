# Documentation

`README.md` is the primary entry point. Everything below is for platform-specific
details, packaging, or maintenance work.

## Core Docs

- [`../README.md`](../README.md): install, run, and configuration overview
- [`../SECURITY.md`](../SECURITY.md): privilege model and operational hardening
- [`../CONTRIBUTING.md`](../CONTRIBUTING.md): local build, test, and CI workflow
- [`PACKAGING.md`](PACKAGING.md): Windows MSI and Linux package/service layout
- [`DESIGN_COMMON.md`](DESIGN_COMMON.md): shared engine contract
- [`DESIGN.md`](DESIGN.md): Windows / WinDivert path
- [`DESIGN_LINUX.md`](DESIGN_LINUX.md): Linux / NFQUEUE path
- [`DESIGN_BSD.md`](DESIGN_BSD.md): FreeBSD / pf divert path
- [`CODESIGNING.md`](CODESIGNING.md): Windows signing flow
- [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md): shipped dependencies and licenses
- [`THIRD_PARTY_SOURCES.md`](THIRD_PARTY_SOURCES.md): vendored source origins

## Supporting Assets

- [`pf/`](pf/): `pf` anchor examples for FreeBSD / pfSense
- [`examples/`](examples/): sample config files
- [`screenshots/`](screenshots/): TUI and icon screenshots

## Archive

Historical planning notes, release checklists, one-off PoC documents, and
deprecated platform notes were moved to [`archive/`](archive/). They are kept
for reference, but they are no longer part of the primary documentation surface.
