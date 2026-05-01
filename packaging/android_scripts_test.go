package packaging_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidBuildScriptsValidateInputs(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path      string
		fragments []string
	}{
		{
			path: filepath.Join("scripts", "android", "build_deps.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`ORIGINAL_DIR="$(pwd -P)"`,
				`absolute_path_arg()`,
				`lookup_trusted_command()`,
				`MAKE_BIN="$(lookup_trusted_command make)"`,
				`GETCONF_BIN="$(lookup_optional_trusted_command getconf || true)"`,
				`ENV_BIN="$(lookup_trusted_command env)"`,
				`[ -n "${GETCONF_BIN:-}" ]`,
				`require_value "$@"`,
				`validate_android_api`,
				`while [ "${api#0}" != "$api" ]; do`,
				`[ "$api" \> "100" ]`,
				`API="$api"`,
				`validate_path_arg "--ndk" "$NDK"`,
				`LIBMNL_SRC="$(absolute_path_arg "$LIBMNL_SRC")"`,
				`NFQ_SRC="$(absolute_path_arg "$NFQ_SRC")"`,
				`OUT="$(absolute_path_arg "$OUT")"`,
				`required tool not executable`,
				`"$MAKE_BIN" -j"$(parallel_jobs)"`,
				`"$ENV_BIN" CC="$CC" AR="$AR"`,
			},
		},
		{
			path: filepath.Join("scripts", "android", "build_splitter_android.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`ORIGINAL_DIR="$(pwd -P)"`,
				`SCRIPT_DIR="$(cd "$script_dir" && pwd -P)"`,
				`REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd -P)"`,
				`absolute_path_arg()`,
				`lookup_go_command()`,
				`GOV_PASS_GO_BIN`,
				`GO_BIN="$(lookup_go_command)"`,
				`require_value "$@"`,
				`validate_android_api`,
				`while [ "${api#0}" != "$api" ]; do`,
				`[ "$api" \> "100" ]`,
				`API="$api"`,
				`validate_path_arg "--deps" "$DEPS"`,
				`DEPS="$(absolute_path_arg "$DEPS")"`,
				`OUT="$(absolute_path_arg "$OUT")"`,
				`deps must contain include and lib directories`,
				`"$MKDIR_BIN" -p "$OUT_DIR"`,
				`cd "$REPO_ROOT"`,
				`"$GO_BIN" build -o "$OUT"`,
			},
		},
		{
			path: filepath.Join("scripts", "android", "build_magisk_module.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`lookup_trusted_command()`,
				`ZIP_BIN="$(lookup_trusted_command zip)"`,
				`MKTEMP_BIN="$(lookup_trusted_command mktemp)"`,
				`CP_BIN="$(lookup_trusted_command cp)"`,
				`FIND_BIN="$(lookup_trusted_command find)"`,
				`refusing symlinked command`,
				`copy_shared_libs()`,
				`refusing symlinked shared library`,
				`no shared libraries found in`,
				`validate_version_value`,
				`validate_path_arg "--template" "$TEMPLATE"`,
				`"$FIND_BIN" "$TMP_DIR" -type l`,
				`refusing Magisk template containing symlinks`,
				`"$RM_BIN" -rf "$TMP_DIR/lib"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, fragment := range tt.fragments {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required hardening fragment %q", tt.path, fragment)
				}
			}
		})
	}
}

func TestAndroidNativeBuildScriptsAvoidAmbientToolLookup(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path      string
		forbidden []string
	}{
		{
			path: filepath.Join("scripts", "android", "build_deps.sh"),
			forbidden: []string{
				`case "$(uname -s)"`,
				`mkdir -p "$OUT"`,
				`make -j"$(parallel_jobs)"`,
				`make install`,
				`env CC="$CC"`,
				`GETCONF_BIN="$(lookup_trusted_command getconf)"`,
				`"$API" -lt 21`,
				`"$API" -gt 100`,
			},
		},
		{
			path: filepath.Join("scripts", "android", "build_splitter_android.sh"),
			forbidden: []string{
				`case "$(uname -s)"`,
				`mkdir -p "$(dirname "$OUT")"`,
				`go build -o "$OUT"`,
				`"$API" -lt 21`,
				`"$API" -gt 100`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still contains ambient tool fragment %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestAndroidMagiskBuildAvoidsAmbientZipLookup(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "android", "build_magisk_module.sh"))
	for _, forbidden := range []string{
		`command -v zip`,
		`zip -r "$OUT_ABS"`,
		`TMP_DIR="$(mktemp -d)"`,
		`rm -rf "$TMP_DIR"`,
		`cp -R "$TEMPLATE"`,
		`find "$TMP_DIR" -type l`,
		`grep -q .`,
		`chmod 0755`,
		`mv "$PROP_FILE.tmp"`,
		`dirname "$OUT"`,
		`basename "$OUT"`,
		`"$CP_BIN" "$LIB_DIR"/*.so "$TMP_DIR/lib/" 2>/dev/null || true`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/android/build_magisk_module.sh still contains ambient zip fragment %q", forbidden)
		}
	}
}

func TestAndroidMagiskScriptsValidateRuntimeInputs(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		filepath.Join("scripts", "android", "magisk", "iptables_add.sh"),
		filepath.Join("scripts", "android", "magisk", "iptables_del.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`TRUSTED_PATH=`,
				`resolve_iptables_command()`,
				`require_value "$@"`,
				`validate_uint "--queue-num" "$QUEUE_NUM" 65535`,
				`validate_uint "--mark" "$MARK" 4294967295`,
				`IPTABLES="$(resolve_iptables_command)"`,
				`PATH="$TRUSTED_PATH" command -v "$value"`,
				`"$IPTABLES" -t mangle`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required hardening fragment %q", script, fragment)
				}
			}
			if strings.Contains(text, `$IPTABLES -t mangle`) {
				t.Fatalf("%s still invokes iptables through an unquoted variable", script)
			}
			if strings.Contains(text, `command -v "$IPTABLES"`) {
				t.Fatalf("%s still checks IPTABLES via direct ambient command lookup", script)
			}
		})
	}

	service := readTextFile(t, filepath.Join(root, "scripts", "android", "magisk", "service.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`PATH="$TRUSTED_PATH"`,
		`*) MODDIR="." ;;`,
		`load_config()`,
		`unsupported config key`,
		`validate_uint "QUEUE_NUM" "$QUEUE_NUM" 65535`,
		`validate_uint "MARK" "$MARK" 4294967295`,
		`${LD_LIBRARY_PATH:-}`,
	} {
		if !strings.Contains(service, fragment) {
			t.Fatalf("service.sh does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`. "$CONFIG"`,
		`EXTRA_ARGS`,
	} {
		if strings.Contains(service, forbidden) {
			t.Fatalf("service.sh still contains unsafe config execution fragment %q", forbidden)
		}
	}

	uninstall := readTextFile(t, filepath.Join(root, "scripts", "android", "magisk", "uninstall.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`lookup_trusted_command()`,
		`CAT_BIN="$(lookup_trusted_command cat)"`,
		`RM_BIN="$(lookup_trusted_command rm)"`,
		`if [ -n "${MODPATH:-}" ]; then`,
		`*) MODDIR="." ;;`,
		`CONFIG="/data/adb/gov-pass.conf"`,
		`load_config()`,
		`unsupported config key`,
		`validate_pid()`,
		`validate_uint "PID" "$value" 4194304`,
		`PID must be a positive integer`,
		`validate_uint "QUEUE_NUM" "$QUEUE_NUM" 65535`,
		`validate_uint "MARK" "$MARK" 4294967295`,
		`pid="$("$CAT_BIN" "$PIDFILE")"`,
		`kill "$pid"`,
		`"$RM_BIN" -f "$PIDFILE"`,
	} {
		if !strings.Contains(uninstall, fragment) {
			t.Fatalf("uninstall.sh does not contain required hardening fragment %q", fragment)
		}
	}
	if strings.Contains(uninstall, `. "/data/adb/gov-pass.conf"`) {
		t.Fatal("uninstall.sh still sources gov-pass.conf as shell")
	}
	if strings.Contains(uninstall, `kill "$(cat "$PIDFILE")"`) {
		t.Fatal("uninstall.sh still kills unvalidated PID file content")
	}
	for _, forbidden := range []string{
		`pid="$(cat "$PIDFILE")"`,
		`rm -f "$PIDFILE"`,
		`KILL_BIN="$(lookup_trusted_command kill)"`,
	} {
		if strings.Contains(uninstall, forbidden) {
			t.Fatalf("uninstall.sh still uses ambient runtime command fragment %q", forbidden)
		}
	}

	postFSData := readTextFile(t, filepath.Join(root, "scripts", "android", "magisk", "post-fs-data.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`lookup_trusted_command()`,
		`MKDIR_BIN="$(lookup_trusted_command mkdir)"`,
		`"$MKDIR_BIN" -p /data/adb`,
	} {
		if !strings.Contains(postFSData, fragment) {
			t.Fatalf("post-fs-data.sh does not contain required hardening fragment %q", fragment)
		}
	}
	if strings.Contains(postFSData, `mkdir -p /data/adb`) {
		t.Fatal("post-fs-data.sh still uses ambient mkdir")
	}
}

func TestAndroidArchiveDocumentsLiteralConfigOnly(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "android", "README.md"))
	for _, fragment := range []string{
		`do not source`,
		`Only literal ` + "`QUEUE_NUM=N`" + ` and ` + "`MARK=N`",
		`arbitrary extra splitter arguments are not supported`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/android/README.md does not document literal config behavior %q", fragment)
		}
	}
}
