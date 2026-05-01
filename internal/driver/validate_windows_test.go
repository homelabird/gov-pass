//go:build windows

package driver

import "testing"

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"WinDivert", false},
		{"gov-pass_1", false},
		{"A", false},
		{"", true},
		{"  ", true},
		{"Win Divert", true},
		{"WinDivert;", true},
		{"../WinDivert", true},
		{`Win\\Divert`, true},
		{`WinDivert!`, true},
	}
	for _, tt := range tests {
		err := ValidateServiceName(tt.name)
		if tt.wantErr && err == nil {
			t.Fatalf("ValidateServiceName(%q) expected error", tt.name)
		}
		if !tt.wantErr && err != nil {
			t.Fatalf("ValidateServiceName(%q) unexpected error: %v", tt.name, err)
		}
	}
}

func TestValidateDriverFileName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", false},
		{"WinDivert64.sys", false},
		{"windivert.sys", false},
		{"WinDivert64.SYS", false},
		{"WinDivert64.sys.bak", true},
		{`C:\temp\WinDivert64.sys`, true},
		{`..\WinDivert64.sys`, true},
		{"WinDivert64", true},
		{"WinDivert64.dll", true},
		{".sys", true},
		{"name .sys", true},
		{"CON.sys", true},
		{"nul.SYS", true},
		{"COM1.sys", true},
		{"LPT9.sys", true},
		{"CONOUT$.sys", true},
	}
	for _, tt := range tests {
		err := ValidateDriverFileName(tt.name)
		if tt.wantErr && err == nil {
			t.Fatalf("ValidateDriverFileName(%q) expected error", tt.name)
		}
		if !tt.wantErr && err != nil {
			t.Fatalf("ValidateDriverFileName(%q) unexpected error: %v", tt.name, err)
		}
	}
}
