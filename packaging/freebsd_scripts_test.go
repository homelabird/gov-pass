package packaging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeBSDOperationalScriptsAreExecutable(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "freebsd", "gov-pass"),
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "uninstall_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-remove-pf.sh"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(root, script))
			if err != nil {
				t.Fatalf("stat %s: %v", script, err)
			}
			if info.Mode()&0111 == 0 {
				t.Fatalf("%s is not executable: mode %v", script, info.Mode())
			}
		})
	}
}

func TestFreeBSDPFScriptsValidateAnchorName(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "uninstall_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-remove-pf.sh"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`validate_anchor_name()`,
				`""|*[!A-Za-z0-9_-]*)`,
				`invalid anchor name`,
				`validate_anchor_name`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required anchor validation fragment %q", script, fragment)
				}
			}
		})
	}
}

func TestFreeBSDPFScriptsValidatePrivilegedPaths(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "uninstall_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-remove-pf.sh"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`validate_absolute_path()`,
				`*[!A-Za-z0-9_./:@+-]*`,
				`*"/../"*|*/..|../*|..)`,
				`must be an absolute path`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required path validation fragment %q", script, fragment)
				}
			}
		})
	}
}

func TestFreeBSDPFScriptsRejectSymlinkedTrustedCommands(t *testing.T) {
	root := repoRoot(t)
	scripts := []string{
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "uninstall_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-remove-pf.sh"),
	}

	for _, script := range scripts {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			for _, fragment := range []string{
				`TRUSTED_PATH=`,
				`lookup_trusted_command()`,
				`[ -L "$candidate" ]`,
				`refusing symlinked trusted command`,
				`PFCTL_BIN="$(lookup_trusted_command pfctl)"`,
				`ID_BIN="$(lookup_trusted_command id)"`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required trusted command fragment %q", script, fragment)
				}
			}
			if strings.Contains(text, `command -v pfctl`) {
				t.Fatalf("%s still checks pfctl via ambient command lookup", script)
			}
		})
	}
}

func TestFreeBSDPFScriptsValidateArgumentsAndAnchorDestination(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "uninstall_pf_anchor.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			if !strings.Contains(text, `need_arg()`) || !strings.Contains(text, `requires a value`) {
				t.Fatalf("%s does not validate option arguments", script)
			}
		})
	}

	for _, script := range []string{
		filepath.Join("scripts", "freebsd", "install_pf_anchor.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			if !strings.Contains(text, `refusing to overwrite symlink anchor destination`) {
				t.Fatalf("%s does not reject symlink anchor destinations", script)
			}
		})
	}

	for _, script := range []string{
		filepath.Join("scripts", "freebsd", "gov-pass-apply-pf.sh"),
		filepath.Join("scripts", "freebsd", "gov-pass-remove-pf.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			if !strings.Contains(text, `ANCHOR_DEST must end with /${ANCHOR_NAME}`) {
				t.Fatalf("%s does not bind ANCHOR_DEST to ANCHOR_NAME", script)
			}
		})
	}
}

func TestFreeBSDRcScriptUsesTrustedCommandsAndValidatedPID(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "freebsd", "gov-pass"))
	for _, fragment := range []string{
		`TRUSTED_PATH=`,
		`PATH="${TRUSTED_PATH}"`,
		`lookup_trusted_command()`,
		`TOUCH_BIN="$(lookup_trusted_command touch)"`,
		`CHMOD_BIN="$(lookup_trusted_command chmod)"`,
		`CAT_BIN="$(lookup_trusted_command cat)"`,
		`RM_BIN="$(lookup_trusted_command rm)"`,
		`DAEMON_BIN="$(lookup_trusted_command daemon)"`,
		`read_pidfile()`,
		`invalid pidfile content`,
		`"${DAEMON_BIN}" -f -p "${pidfile}"`,
		`"${RM_BIN}" -f "${pidfile}"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/freebsd/gov-pass does not contain required hardening fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{
		`touch "${gov_pass_log}"`,
		`chmod 0600 "${gov_pass_log}"`,
		`cat "${pidfile}"`,
		`rm -f "${pidfile}"`,
		`/usr/sbin/daemon -f -p "${pidfile}"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scripts/freebsd/gov-pass still uses ambient command fragment %q", forbidden)
		}
	}
}

func TestFreeBSDInstallPFAnchorRefreshesManagedBlock(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join("scripts", "freebsd", "install_pf_anchor.sh")
	text := readTextFile(t, filepath.Join(root, script))
	for _, fragment := range []string{
		`"$AWK_BIN" -v begin="$MANAGED_BEGIN" -v end="$MANAGED_END"`,
		`"$MV_BIN" "$TMP_STRIPPED" "$TMP_CONF"`,
		`"$GREP_BIN" -Eq "^[[:space:]]*anchor[[:space:]]+\"${ANCHOR_NAME}\"([[:space:]]|$)"`,
		`if [ "$need_anchor_line" -eq 1 ] || [ "$need_load_line" -eq 1 ]; then`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("%s does not contain required managed block refresh fragment %q", script, fragment)
		}
	}
	if strings.Contains(text, `if ! grep -Fq "$MANAGED_BEGIN" "$TMP_CONF"`) {
		t.Fatalf("%s still skips refresh when a stale managed block exists", script)
	}
}

func TestSourceInstallerInstallsFreeBSDOperationalScripts(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "install_one_touch.sh"))
	if !strings.Contains(text, `[ -L "$candidate" ]`) {
		t.Fatal("scripts/install_one_touch.sh does not reject symlinked trusted command candidates")
	}
	for _, fragment := range []string{
		`scripts/freebsd/gov-pass /usr/local/etc/rc.d/gov-pass`,
		`scripts/freebsd/install_pf_anchor.sh /usr/local/libexec/gov-pass/install_pf_anchor.sh`,
		`scripts/freebsd/uninstall_pf_anchor.sh /usr/local/libexec/gov-pass/uninstall_pf_anchor.sh`,
		`scripts/freebsd/gov-pass-apply-pf.sh /usr/local/libexec/gov-pass/gov-pass-apply-pf.sh`,
		`scripts/freebsd/gov-pass-remove-pf.sh /usr/local/libexec/gov-pass/gov-pass-remove-pf.sh`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/install_one_touch.sh does not install FreeBSD operational file %q", fragment)
		}
	}
}
