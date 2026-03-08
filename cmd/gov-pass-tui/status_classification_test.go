package main

import "testing"

func TestClassifySystemctlActiveStatus(t *testing.T) {
	tests := []struct {
		text   string
		state  string
		active bool
		ok     bool
	}{
		{text: "active", state: "active", active: true, ok: true},
		{text: "inactive", state: "inactive", active: false, ok: true},
		{text: "failed", state: "failed", active: false, ok: true},
		{text: "unknown", state: "unknown", active: false, ok: false},
	}

	for _, tt := range tests {
		state, active, ok := classifySystemctlActiveStatus(tt.text)
		if state != tt.state || active != tt.active || ok != tt.ok {
			t.Fatalf("classifySystemctlActiveStatus(%q) = (%q, %t, %t), want (%q, %t, %t)", tt.text, state, active, ok, tt.state, tt.active, tt.ok)
		}
	}
}

func TestClassifySystemctlEnabledStatus(t *testing.T) {
	tests := []struct {
		text    string
		enabled bool
		ok      bool
	}{
		{text: "enabled", enabled: true, ok: true},
		{text: "linked-runtime", enabled: true, ok: true},
		{text: "disabled", enabled: false, ok: true},
		{text: "masked", enabled: false, ok: true},
		{text: "not-found", enabled: false, ok: false},
	}

	for _, tt := range tests {
		enabled, ok := classifySystemctlEnabledStatus(tt.text)
		if enabled != tt.enabled || ok != tt.ok {
			t.Fatalf("classifySystemctlEnabledStatus(%q) = (%t, %t), want (%t, %t)", tt.text, enabled, ok, tt.enabled, tt.ok)
		}
	}
}

func TestClassifyFreeBSDServiceStatusOutput(t *testing.T) {
	tests := []struct {
		text    string
		success bool
		state   string
		active  bool
		ok      bool
	}{
		{text: "", success: true, state: "active", active: true, ok: true},
		{text: "gov-pass is not running as a static service", success: false, state: "inactive", active: false, ok: true},
		{text: "gov-pass is running as pid 1234", success: false, state: "active", active: true, ok: true},
		{text: "service gov-pass does not exist", success: false, state: "unknown", active: false, ok: false},
	}

	for _, tt := range tests {
		state, active, ok := classifyFreeBSDServiceStatusOutput(tt.text, tt.success)
		if state != tt.state || active != tt.active || ok != tt.ok {
			t.Fatalf("classifyFreeBSDServiceStatusOutput(%q, %t) = (%q, %t, %t), want (%q, %t, %t)", tt.text, tt.success, state, active, ok, tt.state, tt.active, tt.ok)
		}
	}
}

func TestClassifyFreeBSDBootSetting(t *testing.T) {
	tests := []struct {
		text    string
		enabled bool
		ok      bool
	}{
		{text: "YES", enabled: true, ok: true},
		{text: "NO", enabled: false, ok: true},
		{text: "", enabled: false, ok: true},
		{text: "maybe", enabled: false, ok: false},
	}

	for _, tt := range tests {
		enabled, ok := classifyFreeBSDBootSetting(tt.text)
		if enabled != tt.enabled || ok != tt.ok {
			t.Fatalf("classifyFreeBSDBootSetting(%q) = (%t, %t), want (%t, %t)", tt.text, enabled, ok, tt.enabled, tt.ok)
		}
	}
}
