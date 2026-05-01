package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConfigSchemasValidateExamples(t *testing.T) {
	schemaDir := filepath.Join("..", "..", "docs", "schema")
	exampleDir := filepath.Join("..", "..", "docs", "examples")

	schemas := loadSchemaDocs(t, schemaDir)
	examples, err := filepath.Glob(filepath.Join(exampleDir, "splitter.*.json"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("no splitter example configs found")
	}

	seenExamples := make(map[string]bool)
	for _, examplePath := range examples {
		platform := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(examplePath), "splitter."), ".json")
		schemaPath := filepath.Clean(filepath.Join(schemaDir, "splitter."+platform+".schema.json"))
		schema, ok := schemas[schemaPath]
		if !ok {
			t.Fatalf("%s has no matching schema %s", examplePath, schemaPath)
		}
		example := decodeJSONFile(t, examplePath)
		if err := validateJSONSchemaSubset(example, schema, schemaPath, schemas, "$"); err != nil {
			t.Fatalf("%s does not match %s: %v", examplePath, schemaPath, err)
		}
		seenExamples[platform] = true
	}

	for schemaPath := range schemas {
		name := filepath.Base(schemaPath)
		if name == "splitter.common.schema.json" {
			continue
		}
		if !strings.HasPrefix(name, "splitter.") || !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		platform := strings.TrimSuffix(strings.TrimPrefix(name, "splitter."), ".schema.json")
		if !seenExamples[platform] {
			t.Fatalf("%s has no matching example config", schemaPath)
		}
	}
}

func TestValidateJSONSchemaSubsetEnforcesStringConstraints(t *testing.T) {
	schema := map[string]any{
		"type":      "string",
		"minLength": json.Number("2"),
		"maxLength": json.Number("3"),
		"pattern":   "^[a-z]+$",
	}

	if err := validateJSONSchemaSubset("abc", schema, "schema.json", nil, "$"); err != nil {
		t.Fatalf("expected valid constrained string: %v", err)
	}
	if err := validateJSONSchemaSubset("a", schema, "schema.json", nil, "$"); err == nil || !strings.Contains(err.Error(), "minLength") {
		t.Fatalf("expected minLength error, got %v", err)
	}
	if err := validateJSONSchemaSubset("abcd", schema, "schema.json", nil, "$"); err == nil || !strings.Contains(err.Error(), "maxLength") {
		t.Fatalf("expected maxLength error, got %v", err)
	}
	if err := validateJSONSchemaSubset("ABC", schema, "schema.json", nil, "$"); err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Fatalf("expected pattern error, got %v", err)
	}
}

func TestDecodeJSONDocumentRejectsDuplicateFields(t *testing.T) {
	_, err := decodeJSONDocument([]byte(`{
		"engine": {
			"split_mode": "tls-hello",
			"split_mode": "immediate"
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), `$.engine: duplicate field "split_mode"`) {
		t.Fatalf("expected duplicate field error, got %v", err)
	}
}

func TestReadJSONConfigFileRejectsOversizedAndNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	oversized := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte(" "), maxJSONConfigFileBytes+1), 0o644); err != nil {
		t.Fatalf("write oversized config: %v", err)
	}
	if _, err := readJSONConfigFile(oversized); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized config error, got %v", err)
	}

	if _, err := readJSONConfigFile(dir); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected non-regular config error, got %v", err)
	}

	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write target config: %v", err)
	}
	link := filepath.Join(dir, "linked.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := readJSONConfigFile(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink config error, got %v", err)
	}
}

func TestReadJSONConfigFileUsesNoFollowOpenOnUnix(t *testing.T) {
	text, err := os.ReadFile("engine_config_file_unix.go")
	if err != nil {
		t.Fatalf("read unix config opener: %v", err)
	}
	for _, fragment := range []string{
		`syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)`,
		`f.Stat()`,
		`config path must not be a symlink`,
		`config path must be a regular file`,
	} {
		if !strings.Contains(string(text), fragment) {
			t.Fatalf("unix config opener does not contain required no-follow fragment %q", fragment)
		}
	}
}

func TestReadJSONConfigFileUsesReparsePointOpenOnWindows(t *testing.T) {
	text, err := os.ReadFile("engine_config_file_windows.go")
	if err != nil {
		t.Fatalf("read windows config opener: %v", err)
	}
	for _, fragment := range []string{
		`windows.CreateFile(`,
		`windows.FILE_FLAG_OPEN_REPARSE_POINT`,
		`windows.FileAttributeTagInfo`,
		`windows.FILE_ATTRIBUTE_REPARSE_POINT`,
		`config path must not be a reparse point`,
	} {
		if !strings.Contains(string(text), fragment) {
			t.Fatalf("windows config opener does not contain required reparse-point fragment %q", fragment)
		}
	}
}

func loadSchemaDocs(t *testing.T, schemaDir string) map[string]any {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(schemaDir, "*.json"))
	if err != nil {
		t.Fatalf("glob schemas: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no config schemas found")
	}

	schemas := make(map[string]any, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		schema := decodeJSONFile(t, clean)
		obj, ok := schema.(map[string]any)
		if !ok {
			t.Fatalf("%s: schema root must be an object", clean)
		}
		if got, ok := obj["$schema"].(string); !ok || got == "" {
			t.Fatalf("%s: missing $schema", clean)
		}
		schemas[clean] = schema
	}
	return schemas
}

func decodeJSONFile(t *testing.T, path string) any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	value, err := decodeJSONDocument(b)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func decodeJSONDocument(data []byte) (any, error) {
	if err := rejectDuplicateJSONFields(data); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	return value, nil
}

func validateJSONSchemaSubset(value any, schema any, schemaPath string, schemas map[string]any, instancePath string) error {
	schemaObj, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema node is not an object", instancePath)
	}
	if ref, ok := schemaObj["$ref"].(string); ok {
		resolved, resolvedPath, err := resolveSchemaRef(ref, schemaPath, schemas)
		if err != nil {
			return fmt.Errorf("%s: %w", instancePath, err)
		}
		return validateJSONSchemaSubset(value, resolved, resolvedPath, schemas, instancePath)
	}
	if err := validateConst(value, schemaObj, instancePath); err != nil {
		return err
	}
	if err := validateEnum(value, schemaObj, instancePath); err != nil {
		return err
	}
	if err := validateAnyOf(value, schemaObj, schemaPath, schemas, instancePath); err != nil {
		return err
	}

	typeName, _ := schemaObj["type"].(string)
	switch typeName {
	case "":
		return nil
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object, got %T", instancePath, value)
		}
		return validateJSONObject(obj, schemaObj, schemaPath, schemas, instancePath)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %T", instancePath, value)
		}
		itemSchema, ok := schemaObj["items"]
		if !ok {
			return nil
		}
		for i, item := range items {
			if err := validateJSONSchemaSubset(item, itemSchema, schemaPath, schemas, fmt.Sprintf("%s[%d]", instancePath, i)); err != nil {
				return err
			}
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", instancePath, value)
		}
		return nil
	case "integer":
		num, ok := value.(json.Number)
		if !ok || !jsonNumberIsInteger(num) {
			return fmt.Errorf("%s: expected integer, got %T", instancePath, value)
		}
		return validateNumberBounds(num, schemaObj, instancePath)
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: expected string, got %T", instancePath, value)
		}
		return validateStringConstraints(text, schemaObj, instancePath)
	default:
		return fmt.Errorf("%s: unsupported schema type %q", instancePath, typeName)
	}
}

func validateJSONObject(obj map[string]any, schemaObj map[string]any, schemaPath string, schemas map[string]any, instancePath string) error {
	props, _ := schemaObj["properties"].(map[string]any)
	required, err := schemaStringSet(schemaObj, "required")
	if err != nil {
		return fmt.Errorf("%s: %w", instancePath, err)
	}
	for name := range required {
		if _, ok := obj[name]; !ok {
			return fmt.Errorf("%s: missing required field %q", instancePath, name)
		}
	}

	additionalAllowed := true
	if v, ok := schemaObj["additionalProperties"].(bool); ok {
		additionalAllowed = v
	}
	for name, fieldValue := range obj {
		fieldSchema, ok := props[name]
		if !ok {
			if additionalAllowed {
				continue
			}
			return fmt.Errorf("%s: unknown field %q", instancePath, name)
		}
		if err := validateJSONSchemaSubset(fieldValue, fieldSchema, schemaPath, schemas, instancePath+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func resolveSchemaRef(ref string, basePath string, schemas map[string]any) (any, string, error) {
	filePart, pointer, _ := strings.Cut(ref, "#")
	targetPath := basePath
	if filePart != "" {
		targetPath = filepath.Clean(filepath.Join(filepath.Dir(basePath), filePart))
	}
	root, ok := schemas[targetPath]
	if !ok {
		return nil, "", fmt.Errorf("schema ref %q targets missing file %s", ref, targetPath)
	}
	node, err := lookupJSONPointer(root, pointer)
	if err != nil {
		return nil, "", fmt.Errorf("schema ref %q: %w", ref, err)
	}
	return node, targetPath, nil
}

func lookupJSONPointer(root any, pointer string) (any, error) {
	if pointer == "" {
		return root, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("unsupported JSON pointer %q", pointer)
	}
	node := root
	for _, part := range strings.Split(pointer[1:], "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		obj, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("pointer %q reached non-object", pointer)
		}
		next, ok := obj[key]
		if !ok {
			return nil, fmt.Errorf("pointer %q missing key %q", pointer, key)
		}
		node = next
	}
	return node, nil
}

func validateEnum(value any, schemaObj map[string]any, instancePath string) error {
	rawEnum, ok := schemaObj["enum"].([]any)
	if !ok {
		return nil
	}
	for _, candidate := range rawEnum {
		if reflect.DeepEqual(value, candidate) {
			return nil
		}
	}
	return fmt.Errorf("%s: value %v is not in enum %v", instancePath, value, rawEnum)
}

func validateConst(value any, schemaObj map[string]any, instancePath string) error {
	constValue, ok := schemaObj["const"]
	if !ok {
		return nil
	}
	if reflect.DeepEqual(value, constValue) {
		return nil
	}
	return fmt.Errorf("%s: value %v does not equal const %v", instancePath, value, constValue)
}

func validateAnyOf(value any, schemaObj map[string]any, schemaPath string, schemas map[string]any, instancePath string) error {
	rawAnyOf, ok := schemaObj["anyOf"].([]any)
	if !ok {
		return nil
	}
	if len(rawAnyOf) == 0 {
		return fmt.Errorf("%s: anyOf must not be empty", instancePath)
	}

	errs := make([]string, 0, len(rawAnyOf))
	for i, candidate := range rawAnyOf {
		if err := validateJSONSchemaSubset(value, candidate, schemaPath, schemas, instancePath); err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Sprintf("anyOf[%d]: %v", i, err))
		}
	}
	return fmt.Errorf("%s: value does not match anyOf (%s)", instancePath, strings.Join(errs, "; "))
}

func validateNumberBounds(num json.Number, schemaObj map[string]any, instancePath string) error {
	value, err := strconv.ParseFloat(num.String(), 64)
	if err != nil {
		return fmt.Errorf("%s: invalid number %q", instancePath, num.String())
	}
	if minimum, ok, err := schemaNumber(schemaObj, "minimum"); err != nil {
		return fmt.Errorf("%s: %w", instancePath, err)
	} else if ok && value < minimum {
		return fmt.Errorf("%s: %s is less than minimum %v", instancePath, num.String(), minimum)
	}
	if maximum, ok, err := schemaNumber(schemaObj, "maximum"); err != nil {
		return fmt.Errorf("%s: %w", instancePath, err)
	} else if ok && value > maximum {
		return fmt.Errorf("%s: %s is greater than maximum %v", instancePath, num.String(), maximum)
	}
	return nil
}

func validateStringConstraints(value string, schemaObj map[string]any, instancePath string) error {
	length := utf8.RuneCountInString(value)
	if minimum, ok, err := schemaInteger(schemaObj, "minLength"); err != nil {
		return fmt.Errorf("%s: %w", instancePath, err)
	} else if ok && length < minimum {
		return fmt.Errorf("%s: string length %d is less than minLength %d", instancePath, length, minimum)
	}
	if maximum, ok, err := schemaInteger(schemaObj, "maxLength"); err != nil {
		return fmt.Errorf("%s: %w", instancePath, err)
	} else if ok && length > maximum {
		return fmt.Errorf("%s: string length %d is greater than maxLength %d", instancePath, length, maximum)
	}

	rawPattern, ok := schemaObj["pattern"]
	if !ok {
		return nil
	}
	pattern, ok := rawPattern.(string)
	if !ok {
		return fmt.Errorf("%s: pattern must be a string", instancePath)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%s: invalid pattern %q: %w", instancePath, pattern, err)
	}
	if !re.MatchString(value) {
		return fmt.Errorf("%s: string does not match pattern %q", instancePath, pattern)
	}
	return nil
}

func schemaNumber(schemaObj map[string]any, key string) (float64, bool, error) {
	raw, ok := schemaObj[key]
	if !ok {
		return 0, false, nil
	}
	num, ok := raw.(json.Number)
	if !ok {
		return 0, false, fmt.Errorf("%s is not numeric", key)
	}
	value, err := strconv.ParseFloat(num.String(), 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s is invalid: %w", key, err)
	}
	return value, true, nil
}

func schemaInteger(schemaObj map[string]any, key string) (int, bool, error) {
	raw, ok := schemaObj[key]
	if !ok {
		return 0, false, nil
	}
	num, ok := raw.(json.Number)
	if !ok {
		return 0, false, fmt.Errorf("%s is not numeric", key)
	}
	value, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s is invalid: %w", key, err)
	}
	if value < 0 {
		return 0, false, fmt.Errorf("%s must be non-negative", key)
	}
	if int64(int(value)) != value {
		return 0, false, fmt.Errorf("%s is too large", key)
	}
	return int(value), true, nil
}

func jsonNumberIsInteger(num json.Number) bool {
	text := num.String()
	if strings.ContainsAny(text, ".eE") {
		return false
	}
	if strings.HasPrefix(text, "-") {
		_, err := strconv.ParseInt(text, 10, 64)
		return err == nil
	}
	_, err := strconv.ParseUint(text, 10, 64)
	return err == nil
}

func schemaStringSet(schemaObj map[string]any, key string) (map[string]struct{}, error) {
	raw, ok := schemaObj[key]
	if !ok {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s contains non-string value", key)
		}
		out[text] = struct{}{}
	}
	return out, nil
}
