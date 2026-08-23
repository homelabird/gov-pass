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
	files           []wixFile
	components      []wixComponent
	componentRefs   []string
	shortcutTargets []string
}

type wixFile struct {
	id     string
	source string
}

type wixComponent struct {
	id   string
	guid string
}

func TestWindowsMSITemplateIntegrity(t *testing.T) {
	root := repoRoot(t)
	doc := readWixTemplate(t, filepath.Join(root, "installer", "windows", "gov-pass.wxs.in"))

	components := make(map[string]struct{}, len(doc.components))
	guids := make(map[string]struct{}, len(doc.components))
	for _, component := range doc.components {
		if component.id == "" || component.guid == "" {
			t.Fatalf("WiX component missing identity: %+v", component)
		}
		if _, exists := components[component.id]; exists {
			t.Fatalf("duplicate WiX component Id %q", component.id)
		}
		components[component.id] = struct{}{}
		if component.guid != "*" {
			guid := strings.ToUpper(component.guid)
			if _, exists := guids[guid]; exists {
				t.Fatalf("duplicate WiX component Guid %q", component.guid)
			}
			guids[guid] = struct{}{}
		}
	}
	for _, ref := range doc.componentRefs {
		if _, exists := components[ref]; !exists {
			t.Fatalf("WiX ComponentRef %q has no matching Component", ref)
		}
	}

	sources := make(map[string]struct{}, len(doc.files))
	fileIDs := make(map[string]struct{}, len(doc.files))
	for _, file := range doc.files {
		if file.id == "" {
			t.Fatalf("WiX file source %q has no Id", file.source)
		}
		if _, exists := fileIDs[file.id]; exists {
			t.Fatalf("duplicate WiX file Id %q", file.id)
		}
		fileIDs[file.id] = struct{}{}
		sources[file.source] = struct{}{}
		if rel, ok := strings.CutPrefix(file.source, "{{SOURCE_DIR}}/"); ok {
			if path, check := wixSourceRepoPath(root, rel); check {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("WiX file %s references missing source %s: %v", file.id, path, err)
				}
			}
		}
	}
	requireGlobbedWixSources(t, root, sources, filepath.Join("docs", "examples", "splitter.*.json"))
	requireGlobbedWixSources(t, root, sources, filepath.Join("docs", "schema", "*"))

	for _, target := range doc.shortcutTargets {
		if target == "[INSTALLFOLDER]gov-pass-tui-admin.cmd" {
			return
		}
	}
	t.Fatal("Windows TUI shortcut does not use the Administrator launcher")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, ".."))
}

func readWixTemplate(t *testing.T, path string) wixDocument {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var syntaxCheck any
	if err := xml.Unmarshal(data, &syntaxCheck); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	doc, err := decodeWixDocument(data)
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
		if err == io.EOF {
			return doc, nil
		}
		if err != nil {
			return wixDocument{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "File":
			doc.files = append(doc.files, wixFile{id: xmlAttr(start.Attr, "Id"), source: xmlAttr(start.Attr, "Source")})
		case "Component":
			doc.components = append(doc.components, wixComponent{id: xmlAttr(start.Attr, "Id"), guid: xmlAttr(start.Attr, "Guid")})
		case "ComponentRef":
			doc.componentRefs = append(doc.componentRefs, xmlAttr(start.Attr, "Id"))
		case "Shortcut":
			doc.shortcutTargets = append(doc.shortcutTargets, xmlAttr(start.Attr, "Target"))
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

func requireGlobbedWixSources(t *testing.T, root string, sources map[string]struct{}, pattern string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob %s: matches=%d err=%v", pattern, len(paths), err)
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
	case "splitter-admin.cmd", "gov-pass-tui-admin.cmd", "service-start-admin.cmd", "service-stop-admin.cmd",
		"service-restart-admin.cmd", "service-reload-admin.cmd":
		return filepath.Join(root, "installer", "windows", rel), true
	default:
		return filepath.Join(root, filepath.FromSlash(rel)), true
	}
}
