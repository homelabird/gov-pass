//go:build linux

package main

import (
	"errors"
	"strings"
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
		{line: "tcp dport 443 # handle -1", want: 0, ok: false},
		{line: "tcp dport 443 # handle 0", want: 0, ok: false},
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
		{
			out:  "default via 10.0.0.1 dev --help proto dhcp metric 100",
			want: "",
		},
	}

	for _, tt := range tests {
		if got := parseRouteDev(tt.out); got != tt.want {
			t.Fatalf("parseRouteDev(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}

func TestNormalizeLinuxIfaceName(t *testing.T) {
	valid := []string{"eth0", "enp0s31f6", "vlan.100", "tun0", "veth0@if2", "br:wan"}
	for _, iface := range valid {
		t.Run(iface, func(t *testing.T) {
			got, err := normalizeLinuxIfaceName(" " + iface + " ")
			if err != nil {
				t.Fatalf("normalizeLinuxIfaceName(%q) unexpected error: %v", iface, err)
			}
			if got != iface {
				t.Fatalf("normalizeLinuxIfaceName(%q) = %q", iface, got)
			}
		})
	}

	invalid := []string{"", "-eth0", "eth/0", `eth\0`, "bad name", strings.Repeat("a", 16)}
	for _, iface := range invalid {
		t.Run(iface, func(t *testing.T) {
			if _, err := normalizeLinuxIfaceName(iface); err == nil {
				t.Fatalf("expected error for iface %q", iface)
			}
		})
	}
}

func TestParseRouteDevs(t *testing.T) {
	out := strings.Join([]string{
		"default via 10.0.0.1 dev eth0 proto dhcp metric 100",
		"default via 10.0.0.1 dev --help proto dhcp metric 100",
		"default via fe80::1 dev eth0 proto ra metric 100",
		"2606:4700:4700::1111 from :: via fe80::2 dev tun0 src 2001:db8::2 metric 10",
		"default via 192.168.50.1 proto dhcp metric 100",
	}, "\n")

	got := parseRouteDevs(out)
	want := []string{"eth0", "tun0"}
	if len(got) != len(want) {
		t.Fatalf("parseRouteDevs len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseRouteDevs[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestDetectEgressInterfaceWith(t *testing.T) {
	type result struct {
		out string
		err error
	}

	tests := []struct {
		name    string
		results map[string]result
		want    string
		wantErr string
	}{
		{
			name: "prefers single ipv4 candidate",
			results: map[string]result{
				"-o -4 route get 1.1.1.1": {out: "1.1.1.1 via 192.168.0.1 dev eth0 src 192.168.0.10"},
			},
			want: "eth0",
		},
		{
			name: "falls back to ipv6 default route",
			results: map[string]result{
				"-o -4 route get 1.1.1.1":              {err: errors.New("network unreachable")},
				"-o -6 route get 2606:4700:4700::1111": {err: errors.New("network unreachable")},
				"-o -4 route show default":             {err: errors.New("no ipv4 default")},
				"-o -6 route show default":             {out: "default via fe80::1 dev eth1 proto ra metric 100"},
			},
			want: "eth1",
		},
		{
			name: "returns ambiguity when families disagree",
			results: map[string]result{
				"-o -4 route get 1.1.1.1":              {out: "1.1.1.1 via 192.168.0.1 dev eth0 src 192.168.0.10"},
				"-o -6 route get 2606:4700:4700::1111": {out: "2606:4700:4700::1111 from :: via fe80::1 dev tun0 src 2001:db8::2 metric 10"},
			},
			wantErr: "multiple candidate egress interfaces (eth0, tun0)",
		},
		{
			name: "returns explicit error when no routes resolve",
			results: map[string]result{
				"-o -4 route get 1.1.1.1":              {err: errors.New("network unreachable")},
				"-o -6 route get 2606:4700:4700::1111": {err: errors.New("network unreachable")},
				"-o -4 route show default":             {out: ""},
				"-o -6 route show default":             {out: ""},
			},
			wantErr: "could not detect egress interface from IPv4/IPv6 route lookups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ string, args ...string) (string, error) {
				if res, ok := tt.results[strings.Join(args, " ")]; ok {
					return res.out, res.err
				}
				return "", errors.New("unexpected probe")
			}

			got, err := detectEgressInterfaceWith("/usr/bin/ip", runner)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("iface = %q, want %q", got, tt.want)
			}
		})
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
