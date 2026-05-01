package main

import (
	"strings"
	"testing"
)

func TestDecodeStrictJSONConfigRejectsDuplicateFields(t *testing.T) {
	var cfg struct {
		Engine *engineJSONConfig `json:"engine,omitempty"`
	}

	err := decodeStrictJSONConfig([]byte(`{
		"engine": {
			"split_mode": "tls-hello",
			"split_mode": "immediate"
		}
	}`), &cfg)
	if err == nil || !strings.Contains(err.Error(), `$.engine: duplicate field "split_mode"`) {
		t.Fatalf("expected nested duplicate field error, got %v", err)
	}
}

func TestDecodeStrictJSONConfigRejectsDuplicateFieldsInArrays(t *testing.T) {
	var cfg struct {
		Engine *engineJSONConfig `json:"engine,omitempty"`
	}

	err := decodeStrictJSONConfig([]byte(`{
		"engine": {
			"policies": [
				{
					"name": "first",
					"name": "second",
					"dst_cidrs": ["203.0.113.0/24"],
					"skip": true
				}
			]
		}
	}`), &cfg)
	if err == nil || !strings.Contains(err.Error(), `$.engine.policies[0]: duplicate field "name"`) {
		t.Fatalf("expected array duplicate field error, got %v", err)
	}
}

func TestDecodeStrictJSONConfigRejectsDuplicateTopLevelFields(t *testing.T) {
	var cfg struct {
		Engine *engineJSONConfig `json:"engine,omitempty"`
	}

	err := decodeStrictJSONConfig([]byte(`{
		"engine": {},
		"engine": {}
	}`), &cfg)
	if err == nil || !strings.Contains(err.Error(), `$: duplicate field "engine"`) {
		t.Fatalf("expected top-level duplicate field error, got %v", err)
	}
}
