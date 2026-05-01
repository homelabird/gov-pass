package packaging_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxNFQueueScriptsUseTrustedLookup(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "linux", "install_nfqueue.sh"),
		filepath.Join("scripts", "linux", "uninstall_nfqueue.sh"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`TRUSTED_PATH=`,
				`PATH="${TRUSTED_PATH}"`,
				`lookup_trusted_command()`,
				`[ -L "$candidate" ]`,
				`require_value "$@"`,
				`10#$value > max`,
				`validate_uint "--queue-num" "$QUEUE_NUM" 65535`,
				`validate_uint "--mark" "$MARK" 4294967295`,
				`ID_BIN="$(lookup_trusted_command id)"`,
				`"$("$ID_BIN" -u)"`,
				`AWK_BIN="$(lookup_trusted_command awk)"`,
				`"$AWK_BIN" -v tag="comment \"$TAG\""`,
				`validate_nft_handle()`,
				`while [ "${value#0}" != "$value" ]; do`,
				`validate_nft_handle "$h" || continue`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required hardening fragment %q", script, fragment)
				}
			}
			for _, forbidden := range []string{
				`if command -v nft`,
				`if command -v iptables`,
				`if command -v ip6tables`,
				`if [ "$(id -u)" -ne 0 ]`,
				`awk -v tag="comment \"$TAG\""`,
			} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still uses ambient PATH lookup %q", script, forbidden)
				}
			}
		})
	}
}

func TestLinuxBootstrapInstallerHardensPathAndInputs(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "install_one_touch_curl.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`PATH="${TRUSTED_PATH}"`,
		`lookup_trusted_command()`,
		`[ -L "$candidate" ]`,
		`CURL_BIN="$(lookup_trusted_command curl)"`,
		`SYSTEMCTL_BIN="$(lookup_trusted_command systemctl)"`,
		`run_privileged()`,
		`"$TAR_BIN" -xzf "$LINUX_RELEASE_PATH" -C "$TMP_DIR"`,
		`Refusing to extract tarball containing non-regular entries.`,
		`substr($0, 1, 1) != "-" && substr($0, 1, 1) != "d"`,
		`run_privileged "$install_bin" -m 0755 "${LINUX_RELEASE_EXTRACTED}/splitter"`,
		`validate_id_component "REPO_OWNER" "$REPO_OWNER"`,
		`validate_id_component "REPO_NAME" "$REPO_NAME"`,
		`validate_release_version "$VERSION"`,
		`validate_tarball_members "$LINUX_RELEASE_PATH" "gov-pass-${VERSION}-linux-amd64"`,
		`/*|../*|*/../*|*/..|..)`,
		`require_release_file "${LINUX_RELEASE_EXTRACTED}/splitter" "splitter binary"`,
		`must not be a symlink`,
		`Release signing public key must not be a symlink`,
		`"$value" == *"/"*`,
		`"$value" == *"\\"*`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/install_one_touch_curl.sh does not contain required hardening fragment %q", fragment)
		}
	}
}

func TestLinuxSourceInstallerFindsGoSafely(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "install_one_touch.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH="/usr/local/go/bin:`,
		`lookup_go_command()`,
		`GOV_PASS_GO_BIN`,
		`validate_explicit_tool_path go "$GOV_PASS_GO_BIN"`,
		`GO_BIN="$(lookup_go_command)"`,
		`"${GO_BIN}" build -o dist/splitter ./cmd/splitter`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/install_one_touch.sh does not contain required Go lookup fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`GO_BIN="$(lookup_trusted_command go)"`,
		`go build -o dist/splitter`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/install_one_touch.sh still uses unsafe Go lookup/build fragment %q", forbidden)
		}
	}
}

func TestLinuxSystemdUnitsHardenSandbox(t *testing.T) {
	root := repoRoot(t)
	for _, file := range []string{
		filepath.Join("scripts", "linux", "gov-pass.service"),
		filepath.Join("packaging", "deb", "gov-pass.service"),
		filepath.Join("packaging", "rpm", "gov-pass.service"),
	} {
		t.Run(file, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, file))
			for _, fragment := range []string{
				`CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW`,
				`AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW`,
				`NoNewPrivileges=true`,
				`ProtectSystem=strict`,
				`ProtectHome=yes`,
				`PrivateTmp=yes`,
				`ProtectKernelTunables=yes`,
				`ProtectKernelModules=yes`,
				`ProtectControlGroups=yes`,
				`MemoryDenyWriteExecute=yes`,
				`LockPersonality=yes`,
				`RestrictNamespaces=yes`,
				`RestrictSUIDSGID=yes`,
				`SystemCallArchitectures=native`,
				`RestrictAddressFamilies=AF_INET AF_INET6 AF_NETLINK AF_PACKET`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required systemd hardening fragment %q", file, fragment)
				}
			}
			if !strings.Contains(text, `$GOV_PASS_ARGS`) {
				t.Fatalf("%s does not split GOV_PASS_ARGS into service argv", file)
			}
			if strings.Contains(text, `${GOV_PASS_ARGS}`) {
				t.Fatalf("%s uses single-argument GOV_PASS_ARGS expansion", file)
			}
		})
	}
}

func TestMakefileDestructiveTargetsGuardPaths(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "Makefile"))
	for _, fragment := range []string{
		`refusing to uninstall unsafe PREFIX`,
		`""|"/"|"/usr"|"/usr/local"|"/opt"|"/etc"|"/bin"|"/sbin"|"/lib"|"/lib64"`,
		`rm -rf "$$prefix"`,
		`refusing to clean unsafe DISTDIR`,
		`""|"/"|"."|".."|../*|*/../*|*/..|/*`,
		`rm -rf "$$distdir"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("Makefile does not contain required destructive target guard %q", fragment)
		}
	}
	if strings.Contains(text, `rm -rf $(DESTDIR)$(PREFIX)`) {
		t.Fatal("Makefile still removes PREFIX without shell guard")
	}
	if strings.Contains(text, `rm -rf $(DISTDIR)`) {
		t.Fatal("Makefile still removes DISTDIR without shell guard")
	}
}

func TestLinuxPCAPVerifyUsesSafeTempOutput(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "linux", "pcap_verify.sh"))
	for _, fragment := range []string{
		`OUT_SET=0`,
		`TRUSTED_PATH=`,
		`PATH="$TRUSTED_PATH"`,
		`lookup_trusted_command()`,
		`refusing symlinked command`,
		`TARGET_URL="https://example.com"`,
		`validate_iface_name`,
		`validate_url`,
		`validate_nonnegative_uint "--wait" "$EXTRA_WAIT"`,
		`--cmd was removed`,
		`"$@" >/dev/null 2>&1 || true`,
		`"$CURL_BIN" -sk "$TARGET_URL" >/dev/null || true`,
		`""|-*|*/*|*\\*|*[!A-Za-z0-9_.:@-]*)`,
		`OUT="$("$MKTEMP_BIN" "${TMP_BASE%/}/gov-pass.pcap.XXXXXX")"`,
		`"$TCPDUMP_BIN" -i "$IFACE"`,
		`refusing to write pcap through symlink`,
		`refusing to overwrite non-regular output path`,
		`trap stop_capture EXIT HUP INT TERM`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/linux/pcap_verify.sh does not contain required hardening fragment %q", fragment)
		}
	}
	if strings.Contains(text, `OUT="/tmp/gov-pass.pcap"`) {
		t.Fatal("scripts/linux/pcap_verify.sh still uses a fixed /tmp output path")
	}
	if strings.Contains(text, `sh -c "$CMD"`) {
		t.Fatal("scripts/linux/pcap_verify.sh still executes a shell command string")
	}
	if strings.Contains(text, `command -v tcpdump`) || strings.Contains(text, `command -v curl`) {
		t.Fatal("scripts/linux/pcap_verify.sh still checks tools via ambient command lookup")
	}
}

func TestLinuxLoadProbeAvoidsShellCurlLoop(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "linux", "load_probe.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`PATH="$TRUSTED_PATH"`,
		`lookup_trusted_command()`,
		`refusing symlinked command`,
		`CURL_BIN="$(lookup_trusted_command curl)"`,
		`validate_positive_uint "--concurrency" "$CONC"`,
		`validate_positive_uint "--requests" "$REQUESTS"`,
		`--target must start with http:// or https://`,
		`"$XARGS_BIN" -0 -n 1 -P "$CONC" "$CURL_BIN"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/linux/load_probe.sh does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`command -v curl`,
		`command -v nstat`,
		`command -v wrk`,
		`command -v hey`,
		`command -v ss`,
		`sh -c "curl`,
		`xargs -I{} -P "$CONC" sh -c`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/linux/load_probe.sh still uses shell curl loop %q", forbidden)
		}
	}
}

func TestLinuxNetnsIntegrationValidatesInputs(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "linux", "netns_integration_test.sh"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`lookup_trusted_command()`,
		`refusing symlinked command`,
		`IP_BIN="$(lookup_trusted_command ip)"`,
		`CURL_BIN="$(lookup_trusted_command curl)"`,
		`MKTEMP_BIN="$(lookup_trusted_command mktemp)"`,
		`validate_uint "--queue-num" "$QUEUE_NUM" 65535`,
		`validate_uint "--mark" "$MARK" 4294967295`,
		`validate_netns_name "$NS"`,
		`"$GREP_BIN" -Fxq -- "$1"`,
		`"$IP_BIN" netns exec "$CLIENT_NS" "$CURL_BIN"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/linux/netns_integration_test.sh does not contain required hardening fragment %q", fragment)
		}
	}
	if strings.Contains(text, `grep -q "^${CLIENT_NS}\b"`) {
		t.Fatal("scripts/linux/netns_integration_test.sh still uses regex netns matching")
	}
	for _, forbidden := range []string{
		`command -v "$cmd"`,
		`command -v nft`,
		`command -v iptables`,
		`command -v ip6tables`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/linux/netns_integration_test.sh still uses ambient command lookup %q", forbidden)
		}
	}
}
