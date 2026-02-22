# Documentation Index

## Docs Policy

- `Stable`: actively maintained and suitable for operational use.
- `Beta`: currently usable, but with known follow-up items.
- `Experimental`: implemented or partially implemented with platform caveats.
- `WIP`: incomplete and not yet complete for production use.
- `Deprecated`: historical/reference only; no maintenance commitment.

- [DESIGN_COMMON.md](DESIGN_COMMON.md) (Stable) - Shared behavior across all split engines.
- [CONTRIBUTING.md](../CONTRIBUTING.md) (Stable) - Dev workflow, build/test commands, and CI notes.
- [SECURITY.md](../SECURITY.md) (Stable) - Privilege model and operational hardening notes.
- [DESIGN.md](DESIGN.md) (Stable) - Windows WinDivert architecture, state machine, and split/inject behavior.
- [DESIGN_LINUX.md](DESIGN_LINUX.md) (Beta) - NFQUEUE pipeline, reinjection flow, and Linux-specific edge cases.
- [DESIGN_BSD.md](DESIGN_BSD.md) (Experimental) - pf divert architecture and BSD-specific assumptions.
- [POC_BSD.md](POC_BSD.md) (Experimental) - FreeBSD VM PoC checklist and pf anchor templates.
- [DESIGN_ANDROID.md](DESIGN_ANDROID.md) (Deprecated) - Historical reference for rooted Android/Magisk work.
- [ROADMAP.md](ROADMAP.md) (Stable) - Phased tasks, defaults, and long-term follow-up items.
- [PACKAGING.md](PACKAGING.md) (Stable) - Build outputs, distribution layout, and install/run guidance.
- [CODESIGNING.md](CODESIGNING.md) (Stable) - Windows Authenticode signing in CI (EXE/MSI) and required variables.
- [RELEASE_NOTES.md](RELEASE_NOTES.md) (Stable) - Versioned changes and release notes history.
- [RELEASE_CHECKLIST_LINUX.md](RELEASE_CHECKLIST_LINUX.md) (Beta) - Linux release DoD checklist and validation steps.
- [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) (Stable) - Third-party components and license attributions.
- [THIRD_PARTY_SOURCES.md](THIRD_PARTY_SOURCES.md) (Stable) - Vendored source origins and local patch list.
