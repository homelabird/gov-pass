# Detailed Design - Android Root NFQUEUE (Magisk, arm64)

## Assumptions

- Android arm64 only
- Rooted device with custom kernel
- netfilter_queue enabled in kernel
- iptables available
- SELinux permissive or policy adjusted

## Build pipeline (NDK + cgo)

1) Cross-compile dependencies:
- libmnl
- libnetfilter_queue

2) Build Go binary:
- GOOS=android GOARCH=arm64 CGO_ENABLED=1
- CC/CXX set to NDK clang
- Linker flags point to libmnl/libnetfilter_queue

Note: github.com/florianl/go-nfqueue and github.com/mdlayher/netlink are
linux-only by build tags. Android requires a fork or patch to add android
build tags or an alternative netlink binding.

## Runtime architecture

iptables mangle OUTPUT:
- bypass packets with MARK
- queue tcp dport 443 to NFQUEUE

Engine:
- NFQUEUE receive -> reassembly -> split/inject -> drop original
- fail-open on any error
- SO_MARK set on raw socket reinjection

## Magisk module layout

Expected module tree:

```
module.prop
service.sh
post-fs-data.sh
uninstall.sh
iptables_add.sh
iptables_del.sh
splitter
lib/
  libmnl.so
  libnetfilter_queue.so
```

## Configuration

Optional config file: `/data/adb/gov-pass.conf`

Example:
```
QUEUE_NUM=100
MARK=1
EXTRA_ARGS="--split-mode tls-hello --split-chunk 5"
```

## Logging

Suggested log path: `/data/adb/gov-pass.log`

## Open questions

- Minimum Android version for stable netfilter behavior
- SELinux policy strategy
- go-nfqueue/netlink android support path (fork vs replacement)
