package main

import (
	"testing"

	"fk-gov/internal/engine"
)

func TestParseSplitMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    engine.SplitMode
		wantErr bool
	}{
		{name: "tls-hello", input: "tls-hello", want: engine.SplitModeTLSHello},
		{name: "immediate", input: "immediate", want: engine.SplitModeImmediate},
		{name: "case-insensitive", input: "TLS-HELLO", want: engine.SplitModeTLSHello},
		{name: "trimmed", input: " immediate ", want: engine.SplitModeImmediate},
		{name: "invalid", input: "bad", want: engine.SplitModeTLSHello, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSplitMode(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("mode mismatch: got %v want %v", got, tt.want)
			}
		})
	}
}
