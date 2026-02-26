//go:build linux

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

func TestParseNftHandle(t *testing.T) {
	tests := []struct {
		line string
		want int
		ok   bool
	}{
		{line: "tcp dport 443 queue num 100 bypass comment \"gov-pass\" # handle 17", want: 17, ok: true},
		{line: "tcp dport 443 # handle not-a-number", want: 0, ok: false},
		{line: "tcp dport 443 comment \"gov-pass\"", want: 0, ok: false},
	}

	for _, tt := range tests {
		got, ok := parseNftHandle(tt.line)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("parseNftHandle(%q) = (%d,%v), want (%d,%v)", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseEthtoolOnOff(t *testing.T) {
	tests := []struct {
		line string
		want bool
		ok   bool
	}{
		{line: "generic-receive-offload: on", want: true, ok: true},
		{line: "tcp-segmentation-offload: off", want: false, ok: true},
		{line: "generic-segmentation-offload: on [fixed]", want: true, ok: true},
		{line: "badline", want: false, ok: false},
		{line: "generic-segmentation-offload: maybe", want: false, ok: false},
	}

	for _, tt := range tests {
		got, ok := parseEthtoolOnOff(tt.line)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("parseEthtoolOnOff(%q) = (%v,%v), want (%v,%v)", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseRouteDev(t *testing.T) {
	tests := []struct {
		out  string
		want string
	}{
		{
			out:  "1.1.1.1 via 192.168.0.1 dev enp0s31f6 src 192.168.0.10 uid 1000",
			want: "enp0s31f6",
		},
		{
			out:  "default via 10.0.0.1 dev eth0 proto dhcp metric 100",
			want: "eth0",
		},
		{
			out:  "default via 10.0.0.1 proto dhcp metric 100",
			want: "",
		},
	}

	for _, tt := range tests {
		if got := parseRouteDev(tt.out); got != tt.want {
			t.Fatalf("parseRouteDev(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}

func TestIproutePackageName(t *testing.T) {
	if got := iproutePackageName("dnf"); got != "iproute" {
		t.Fatalf("dnf package mismatch: %q", got)
	}
	if got := iproutePackageName("yum"); got != "iproute" {
		t.Fatalf("yum package mismatch: %q", got)
	}
	if got := iproutePackageName("apt-get"); got != "iproute2" {
		t.Fatalf("apt package mismatch: %q", got)
	}
}
