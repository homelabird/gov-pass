# Detailed Design - Android Root NFQUEUE (Magisk, arm64)

## Assumptions

- Android arm64 only
- Rooted device with custom kernel
- netfilter_queue enabled in kernel
- iptables available
- SELinux permissive or policy adjusted

## Build pipeline (NDK + cgo)

### 1) Cross-compile dependencies

Dependencies:
- libmnl
- libnetfilter_queue

Script (host build machine):
```
./scripts/android/build_deps.sh \
  --ndk $ANDROID_NDK_HOME \
  --api 28 \
  --libmnl-src /path/to/libmnl \
  --nfq-src /path/to/libnetfilter_queue \
  --out third_party/android/arm64
```

Outputs:
- `third_party/android/arm64/include/*`
- `third_party/android/arm64/lib/libmnl.so`
- `third_party/android/arm64/lib/libnetfilter_queue.so`

### 2) Build Go binary (arm64)

```
./scripts/android/build_splitter_android.sh \
  --ndk $ANDROID_NDK_HOME \
  --api 28 \
  --deps third_party/android/arm64 \
  --out dist/android/arm64/splitter
```

### 3) go-nfqueue/netlink android build tags

Upstream `go-nfqueue` and `mdlayher/netlink` are linux-only by build tags.
This repo ships local forks with `android` build tags added:
- `third_party/go-nfqueue`
- `third_party/netlink`

`go.mod` uses:
```
replace github.com/florianl/go-nfqueue => ./third_party/go-nfqueue
replace github.com/mdlayher/netlink => ./third_party/netlink
```

These forks only change build tags and keep original licenses intact.

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

### Magisk packaging

```
./scripts/android/build_magisk_module.sh \
  --template scripts/android/magisk \
  --splitter dist/android/arm64/splitter \
  --lib-dir third_party/android/arm64/lib \
  --out dist/gov-pass-magisk.zip \
  --version 0.1.0 \
  --version-code 1
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
