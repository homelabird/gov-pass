package packaging_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCISigningScriptsUsePrivateUmask(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		filepath.Join("scripts", "ci", "sign_release_manifests.sh"),
		filepath.Join("scripts", "ci", "sign_windows_artifacts.sh"),
		filepath.Join("scripts", "ci", "verify_release_manifest_signature.sh"),
	} {
		t.Run(script, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, script))
			if !strings.Contains(text, "umask 077") {
				t.Fatalf("%s does not set private umask before writing signing material", script)
			}
		})
	}
}

func TestCISigningScriptsRejectSymlinkInputs(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path      string
		fragments []string
	}{
		{
			path: filepath.Join("scripts", "ci", "sign_release_manifests.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`lookup_trusted_command()`,
				`OPENSSL_BIN="$(lookup_trusted_command openssl)"`,
				`BASE64_BIN="$(lookup_trusted_command base64)"`,
				`refusing symlinked trusted command`,
				`[[ -L "$key_path" ]]`,
				`refusing to use symlink private key`,
				`[[ -L "$manifest" ]]`,
				`refusing to sign symlink manifest`,
				`[[ -L "$sig_path" ]]`,
				`refusing to write signature through symlink`,
				`"$RM_BIN" -rf -- "$tmpdir"`,
			},
		},
		{
			path: filepath.Join("scripts", "ci", "sign_windows_artifacts.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`lookup_trusted_command()`,
				`OSSLSIGNCODE_BIN="$(lookup_trusted_command osslsigncode)"`,
				`BASE64_BIN="$(lookup_trusted_command base64)"`,
				`refusing symlinked trusted command`,
				`[[ -L "$f" ]]`,
				`refusing to sign through symlink`,
				`"$RM_BIN" -rf -- "$tmpdir"`,
				`"$MV_BIN" -f -- "$out" "$in"`,
			},
		},
		{
			path: filepath.Join("scripts", "ci", "verify_release_manifest_signature.sh"),
			fragments: []string{
				`TRUSTED_PATH=`,
				`lookup_trusted_command()`,
				`OPENSSL_BIN="$(lookup_trusted_command openssl)"`,
				`BASE64_BIN="$(lookup_trusted_command base64)"`,
				`refusing symlinked trusted command`,
				`[[ -L "$manifest" ]]`,
				`refusing to verify symlink manifest`,
				`[[ -L "$sig_path" ]]`,
				`refusing to verify symlink signature`,
				`[[ -L "$pubkey_path" ]]`,
				`refusing to verify with symlink public key`,
				`"$RM_BIN" -rf -- "$tmpdir"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, fragment := range tt.fragments {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required symlink guard fragment %q", tt.path, fragment)
				}
			}
		})
	}
}

func TestCISigningScriptsAvoidDirectToolInvocations(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path      string
		forbidden []string
	}{
		{
			path: filepath.Join("scripts", "ci", "sign_release_manifests.sh"),
			forbidden: []string{
				`tmpdir="$(mktemp -d)"`,
				`cleanup() { rm -rf -- "$tmpdir"; }`,
				`base64 -d`,
				`chmod 600`,
				`openssl pkey`,
				`openssl dgst`,
			},
		},
		{
			path: filepath.Join("scripts", "ci", "verify_release_manifest_signature.sh"),
			forbidden: []string{
				`tmpdir="$(mktemp -d)"`,
				`cleanup() { rm -rf -- "$tmpdir"; }`,
				`base64 -d`,
				`openssl pkey`,
				`openssl dgst`,
			},
		},
		{
			path: filepath.Join("scripts", "ci", "sign_windows_artifacts.sh"),
			forbidden: []string{
				`tmpdir="$(mktemp -d)"`,
				`cleanup() { rm -rf -- "$tmpdir"; }`,
				`base64 -d`,
				`chmod 600`,
				`osslsigncode sign`,
				`mv -f -- "$out" "$in"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still contains direct tool invocation %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestWindowsCodesignScriptValidatesURLs(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "scripts", "ci", "sign_windows_artifacts.sh"))
	for _, fragment := range []string{
		`validate_http_url()`,
		`WINDOWS_CODESIGN_URL`,
		`timestamp URL`,
		`http://*|https://*`,
		`url_args=(-i "$url")`,
		`"${url_args[@]}"`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("scripts/ci/sign_windows_artifacts.sh does not contain URL validation fragment %q", fragment)
		}
	}
	if strings.Contains(text, `${url:+-i "$url"}`) {
		t.Fatal("scripts/ci/sign_windows_artifacts.sh still expands WINDOWS_CODESIGN_URL inline")
	}
}

func TestReleaseWorkflowsValidateVersionBeforeUsingPaths(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		filepath.Join(".github", "workflows", "release.yml"),
		".gitlab-ci.yml",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, file))
			for _, fragment := range []string{
				`validate_release_version()`,
				`*/*|*\\*`,
				`^v?[0-9][0-9A-Za-z._+-]*$`,
			} {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain release version validation fragment %q", file, fragment)
				}
			}
		})
	}
}
