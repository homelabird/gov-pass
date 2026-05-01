package packaging_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type wixDocument struct {
	Files         []wixFile         `xml:".//{http://schemas.microsoft.com/wix/2006/wi}File"`
	Components    []wixComponent    `xml:".//{http://schemas.microsoft.com/wix/2006/wi}Component"`
	ComponentRefs []wixComponentRef `xml:".//{http://schemas.microsoft.com/wix/2006/wi}ComponentRef"`
}

type wixFile struct {
	ID     string `xml:"Id,attr"`
	Source string `xml:"Source,attr"`
}

type wixComponent struct {
	ID   string `xml:"Id,attr"`
	GUID string `xml:"Guid,attr"`
}

type wixComponentRef struct {
	ID string `xml:"Id,attr"`
}

func TestWindowsMSITemplateIncludesConfigDocs(t *testing.T) {
	root := repoRoot(t)
	doc := readWixTemplate(t, root)
	sources := wixSourceSet(doc)

	requireGlobbedWixSources(t, root, sources, filepath.Join("docs", "examples", "splitter.*.json"))
	requireGlobbedWixSources(t, root, sources, filepath.Join("docs", "schema", "*"))
}

func TestWindowsMSITemplateReferencesValidRepoInputs(t *testing.T) {
	root := repoRoot(t)
	doc := readWixTemplate(t, root)
	for _, file := range doc.Files {
		rel, ok := strings.CutPrefix(file.Source, "{{SOURCE_DIR}}/")
		if !ok {
			continue
		}
		path, check := wixSourceRepoPath(root, rel)
		if !check {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("WiX file %s references missing source %s mapped to %s: %v", file.ID, rel, path, err)
		}
	}
}

func TestWindowsMSITemplateComponentRefs(t *testing.T) {
	root := repoRoot(t)
	doc := readWixTemplate(t, root)
	components := make(map[string]string, len(doc.Components))
	seenGUIDs := make(map[string]string, len(doc.Components))
	for _, component := range doc.Components {
		if component.ID == "" {
			t.Fatal("WiX component missing Id")
		}
		if prev, ok := components[component.ID]; ok {
			t.Fatalf("duplicate WiX component Id %q: %s and %s", component.ID, prev, component.GUID)
		}
		components[component.ID] = component.GUID
		if component.GUID == "" {
			t.Fatalf("WiX component %s missing Guid", component.ID)
		}
		if component.GUID == "*" {
			continue
		}
		normalized := strings.ToUpper(component.GUID)
		if prev, ok := seenGUIDs[normalized]; ok {
			t.Fatalf("duplicate WiX component Guid %s on %s and %s", component.GUID, prev, component.ID)
		}
		seenGUIDs[normalized] = component.ID
	}

	for _, ref := range doc.ComponentRefs {
		if _, ok := components[ref.ID]; !ok {
			t.Fatalf("WiX ComponentRef %q has no matching Component", ref.ID)
		}
	}
}

func TestWindowsMSITemplateFileIDsAreUnique(t *testing.T) {
	root := repoRoot(t)
	doc := readWixTemplate(t, root)
	seen := make(map[string]string, len(doc.Files))
	for _, file := range doc.Files {
		if file.ID == "" {
			t.Fatalf("WiX file source %s missing Id", file.Source)
		}
		if prev, ok := seen[file.ID]; ok {
			t.Fatalf("duplicate WiX file Id %q: %s and %s", file.ID, prev, file.Source)
		}
		seen[file.ID] = file.Source
	}
}

func TestReleasePackagingIncludesConfigDocs(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name      string
		path      string
		fragments []string
	}{
		{
			name: "github release tar zip staging",
			path: filepath.Join(".github", "workflows", "release.yml"),
			fragments: []string{
				`mkdir -p "$dest/docs/examples" "$dest/docs/schema" "$dest/licenses"`,
				`cp docs/examples/splitter.*.json "$dest/docs/examples/"`,
				`cp docs/schema/* "$dest/docs/schema/"`,
			},
		},
		{
			name: "github freebsd release operational files",
			path: filepath.Join(".github", "workflows", "release.yml"),
			fragments: []string{
				`mkdir -p "$FREEBSD_DIR/scripts/freebsd" "$FREEBSD_DIR/docs/pf"`,
				`cp scripts/freebsd/gov-pass scripts/freebsd/install_pf_anchor.sh scripts/freebsd/uninstall_pf_anchor.sh scripts/freebsd/gov-pass-apply-pf.sh scripts/freebsd/gov-pass-remove-pf.sh "$FREEBSD_DIR/scripts/freebsd/"`,
				`cp docs/pf/*.conf "$FREEBSD_DIR/docs/pf/"`,
			},
		},
		{
			name: "gitlab release tar zip staging",
			path: ".gitlab-ci.yml",
			fragments: []string{
				`mkdir -p "$dest/docs/examples" "$dest/docs/schema" "$dest/licenses";`,
				`cp docs/examples/splitter.*.json "$dest/docs/examples/";`,
				`cp docs/schema/* "$dest/docs/schema/";`,
			},
		},
		{
			name: "gitlab freebsd release operational files",
			path: ".gitlab-ci.yml",
			fragments: []string{
				`mkdir -p "$FREEBSD_DIR/scripts/freebsd" "$FREEBSD_DIR/docs/pf"`,
				`cp scripts/freebsd/gov-pass scripts/freebsd/install_pf_anchor.sh scripts/freebsd/uninstall_pf_anchor.sh scripts/freebsd/gov-pass-apply-pf.sh scripts/freebsd/gov-pass-remove-pf.sh "$FREEBSD_DIR/scripts/freebsd/"`,
				`cp docs/pf/*.conf "$FREEBSD_DIR/docs/pf/"`,
			},
		},
		{
			name: "deb package docs",
			path: filepath.Join("packaging", "deb", "build_deb.sh"),
			fragments: []string{
				`validate_package_component "version" "$VERSION"`,
				`validate_package_component "architecture" "$ARCH"`,
				`DIST_DIR="$ROOT_DIR/dist"`,
				`validate_dist_dir()`,
				`refusing to use symlink dist directory`,
				`validate_stage_path()`,
				`refusing to remove unsafe package stage directory`,
				`validate_deb_output_path "$OUT_DEB"`,
				`lookup_trusted_command()`,
				`lookup_go_command()`,
				`GOV_PASS_GO_BIN`,
				`refusing symlinked trusted command`,
				`GO_BIN="$(lookup_go_command)"`,
				`DPKG_DEB_BIN="$(lookup_trusted_command dpkg-deb)"`,
				`"$RM_BIN" -rf -- "$STAGE_DIR"`,
				`"$RM_BIN" -f -- "$OUT_DEB"`,
				`PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"`,
				`SYSTEMCTL_BIN="$(lookup_optional_trusted_command systemctl || true)"`,
				`"$STAGE_DIR/usr/share/doc/gov-pass/examples"`,
				`"$STAGE_DIR/usr/share/doc/gov-pass/schema"`,
				`"$INSTALL_BIN" -m 0644 docs/examples/splitter.*.json "$STAGE_DIR/usr/share/doc/gov-pass/examples/"`,
				`"$INSTALL_BIN" -m 0644 docs/schema/* "$STAGE_DIR/usr/share/doc/gov-pass/schema/"`,
			},
		},
		{
			name: "rpm package docs",
			path: filepath.Join("packaging", "rpm", "gov-pass.spec"),
			fragments: []string{
				"%doc README.md SECURITY.md docs/DESIGN.md docs/THIRD_PARTY_NOTICES.md docs/examples docs/schema",
				`TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"`,
				`PATH="$TRUSTED_PATH"`,
				`lookup_optional_trusted_command()`,
				`NFT_BIN="$(lookup_optional_trusted_command nft || true)"`,
				`IPTABLES_BIN="$(lookup_optional_trusted_command iptables || true)"`,
			},
		},
		{
			name: "powershell package docs",
			path: filepath.Join("scripts", "package.ps1"),
			fragments: []string{
				`$schemaSrc = Join-Path $repoRoot "docs\\schema"`,
				`$examplesSrc = Join-Path $repoRoot "docs\\examples"`,
				`Get-ChildItem -LiteralPath $schemaSrc -File`,
				`Get-ChildItem -LiteralPath $examplesSrc -Filter "splitter.*.json" -File`,
				`Copy-SafeFile $_.FullName (Join-Path $schemaOut $_.Name) $_.Name`,
				`Copy-SafeFile $_.FullName (Join-Path $examplesOut $_.Name) $_.Name`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, fragment := range tt.fragments {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required packaging fragment %q", tt.path, fragment)
				}
			}
		})
	}
}

func TestDebBuildScriptAvoidsAmbientCommandLookup(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "packaging", "deb", "build_deb.sh"))
	for _, forbidden := range []string{
		`command -v`,
		`go build -o`,
		`dpkg-deb --build`,
		`systemctl daemon-reload`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("packaging/deb/build_deb.sh still contains ambient command fragment %q", forbidden)
		}
	}
}

func TestRPMSpecAvoidsDirectFirewallCommandLookup(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "packaging", "rpm", "gov-pass.spec"))
	for _, forbidden := range []string{
		`command -v nft`,
		`command -v iptables`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("packaging/rpm/gov-pass.spec still contains direct firewall lookup %q", forbidden)
		}
	}
}

func TestReleaseWorkflowsGuardDestructivePackageDirs(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path      string
		fragments []string
		forbidden []string
	}{
		{
			path: filepath.Join(".github", "workflows", "release.yml"),
			fragments: []string{
				`safe_remove_dir()`,
				`Refusing to remove unsafe directory`,
				`Refusing to remove symlink directory`,
				`safe_remove_dir "$MSI_ROOT" "$BASE_DIR"`,
				`safe_remove_dir "$RPM_TOP" "$GITHUB_WORKSPACE"`,
			},
			forbidden: []string{
				`rm -rf "$MSI_ROOT"`,
				`rm -rf "$RPM_TOP"`,
			},
		},
		{
			path: ".gitlab-ci.yml",
			fragments: []string{
				`safe_remove_dir()`,
				`Refusing to remove unsafe directory`,
				`Refusing to remove symlink directory`,
				`safe_remove_dir "$MSI_ROOT" "$BASE_DIR"`,
				`safe_remove_dir "$RPM_TOP" "$CI_PROJECT_DIR"`,
			},
			forbidden: []string{
				`rm -rf "$MSI_ROOT"`,
				`rm -rf "$RPM_TOP"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			for _, fragment := range tt.fragments {
				if !strings.Contains(text, fragment) {
					t.Fatalf("%s does not contain required destructive directory guard fragment %q", tt.path, fragment)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(text, forbidden) {
					t.Fatalf("%s still contains unguarded destructive directory removal %q", tt.path, forbidden)
				}
			}
		})
	}
}

func TestReleaseWorkflowsPassGoBinaryToDebBuild(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		path     string
		fragment string
	}{
		{
			path:     filepath.Join(".github", "workflows", "release.yml"),
			fragment: `GOV_PASS_GO_BIN="$(command -v go)" bash packaging/deb/build_deb.sh "$RAW_TAG"`,
		},
		{
			path:     ".gitlab-ci.yml",
			fragment: `GOV_PASS_GO_BIN="$(command -v go)" bash packaging/deb/build_deb.sh "$RAW_TAG"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, tt.path))
			if !strings.Contains(text, tt.fragment) {
				t.Fatalf("%s does not pass explicit Go binary to DEB build", tt.path)
			}
		})
	}
}

func TestREADMEDocumentsGoBinaryOverride(t *testing.T) {
	root := repoRoot(t)
	text := readTextFile(t, filepath.Join(root, "README.md"))
	for _, fragment := range []string{
		`GOV_PASS_GO_BIN`,
		`sudo GOV_PASS_GO_BIN="$(command -v go)" ./scripts/install_one_touch.sh`,
		`$env:GOV_PASS_GO_BIN`,
		`absolute, non-symlinked`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("README.md does not document Go binary override fragment %q", fragment)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, ".."))
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func readWixTemplate(t *testing.T, root string) wixDocument {
	t.Helper()
	path := filepath.Join(root, "installer", "windows", "gov-pass.wxs.in")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var syntaxCheck any
	if err := xml.Unmarshal(b, &syntaxCheck); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	doc, err := decodeWixDocument(b)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return doc
}

func decodeWixDocument(data []byte) (wixDocument, error) {
	var doc wixDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return doc, nil
			}
			return wixDocument{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "File":
			doc.Files = append(doc.Files, wixFile{
				ID:     xmlAttr(start.Attr, "Id"),
				Source: xmlAttr(start.Attr, "Source"),
			})
		case "Component":
			doc.Components = append(doc.Components, wixComponent{
				ID:   xmlAttr(start.Attr, "Id"),
				GUID: xmlAttr(start.Attr, "Guid"),
			})
		case "ComponentRef":
			doc.ComponentRefs = append(doc.ComponentRefs, wixComponentRef{
				ID: xmlAttr(start.Attr, "Id"),
			})
		}
	}
}

func xmlAttr(attrs []xml.Attr, localName string) string {
	for _, attr := range attrs {
		if attr.Name.Local == localName {
			return attr.Value
		}
	}
	return ""
}

func wixSourceSet(doc wixDocument) map[string]struct{} {
	sources := make(map[string]struct{}, len(doc.Files))
	for _, file := range doc.Files {
		sources[file.Source] = struct{}{}
	}
	return sources
}

func requireGlobbedWixSources(t *testing.T, root string, sources map[string]struct{}, pattern string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(paths) == 0 {
		t.Fatalf("glob %s matched no files", pattern)
	}
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		source := "{{SOURCE_DIR}}/" + filepath.ToSlash(rel)
		if _, ok := sources[source]; !ok {
			t.Fatalf("WiX template does not include %s", source)
		}
	}
}

func wixSourceRepoPath(root string, rel string) (string, bool) {
	switch rel {
	case "splitter.exe", "gov-pass-tui.exe", "gov-pass-msi-helper.exe",
		"WinDivert.dll", "WinDivert64.sys",
		"licenses/WinDivert-LICENSE.txt", "licenses/go-nfqueue-LICENSE.txt", "licenses/netlink-LICENSE.txt":
		return "", false
	case "splitter-admin.cmd", "service-start-admin.cmd", "service-stop-admin.cmd",
		"service-restart-admin.cmd", "service-reload-admin.cmd":
		return filepath.Join(root, "installer", "windows", rel), true
	default:
		return filepath.Join(root, filepath.FromSlash(rel)), true
	}
}
