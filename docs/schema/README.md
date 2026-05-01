# Splitter JSON Schemas

These schemas document the supported `splitter --config` JSON shape.
Runtime parsing is strict: unknown fields are rejected.

- `splitter.linux.schema.json`: Linux NFQUEUE config
- `splitter.windows.schema.json`: Windows WinDivert config
- `splitter.freebsd.schema.json`: FreeBSD pf divert config
- `splitter.common.schema.json`: shared engine and policy definitions

Examples live under [`../examples`](../examples). The Go test suite parses the
schemas and validates every example against the matching platform schema.
