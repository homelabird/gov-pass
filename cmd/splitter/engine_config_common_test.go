package main

import (
	"strings"
	"testing"
)

func TestDecodeStrictJSONConfigRejectsDuplicateField(t *testing.T) {
	var cfg struct {
		Engine *engineJSONConfig `json:"engine,omitempty"`
	}

	err := decodeStrictJSONConfig([]byte(`{
		"engine": {"policies": [{"name": "first", "name": "second"}]}
	}`), &cfg)
	if err == nil || !strings.Contains(err.Error(), `$.engine.policies[0]: duplicate field "name"`) {
		t.Fatalf("expected duplicate field error, got %v", err)
	}
}
