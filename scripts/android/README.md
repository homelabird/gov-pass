# Android Archive

This directory is kept for historical Android/Magisk experiments only.

Current status:

- Android is not a maintained build target for this repository.
- Android artifacts are not produced by CI or release workflows.
- Scripts in this directory are not covered by runtime E2E tests.
- The active Linux path uses the pure-Go NFQUEUE adapter; these Android scripts
  still reflect an older NDK/cgo-oriented approach described in
  `docs/archive/DESIGN_ANDROID.md`.

Treat these files as reference material unless Android support is explicitly
reopened with a maintained entrypoint, CI smoke tests, and release ownership.

The archived Magisk runtime scripts intentionally do not source
`/data/adb/gov-pass.conf` as shell. Only literal `QUEUE_NUM=N` and `MARK=N`
lines are accepted; arbitrary extra splitter arguments are not supported in
this archived path.
