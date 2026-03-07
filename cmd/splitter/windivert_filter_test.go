package main

import "testing"

func TestUpgradeLegacyWinDivertFilter(t *testing.T) {
	got := upgradeLegacyWinDivertFilter("  outbound   and IP and tcp.DstPort == 443 ")
	if got != defaultWinDivertFilter {
		t.Fatalf("legacy filter not upgraded: got %q want %q", got, defaultWinDivertFilter)
	}
}

func TestUpgradeLegacyWinDivertFilter_PreservesCustom(t *testing.T) {
	custom := "outbound and ipv6 and tcp.DstPort == 443 and tcp.PayloadLength > 0"
	if got := upgradeLegacyWinDivertFilter(custom); got != custom {
		t.Fatalf("custom filter changed: got %q want %q", got, custom)
	}
}

func TestDefaultWinDivertFilterIncludesIPv6(t *testing.T) {
	if want := "ipv6"; !containsCanonicalToken(defaultWinDivertFilter, want) {
		t.Fatalf("default filter must include %q, got %q", want, defaultWinDivertFilter)
	}
	if want := "ip"; !containsCanonicalToken(defaultWinDivertFilter, want) {
		t.Fatalf("default filter must include %q, got %q", want, defaultWinDivertFilter)
	}
}

func containsCanonicalToken(filter string, token string) bool {
	canonical := canonicalizeWinDivertFilter(filter)
	for _, field := range splitCanonicalFilter(canonical) {
		if field == token {
			return true
		}
	}
	return false
}

func splitCanonicalFilter(filter string) []string {
	if filter == "" {
		return nil
	}
	fields := make([]string, 0, len(filter))
	start := 0
	for i := 0; i < len(filter); i++ {
		switch filter[i] {
		case ' ', '(', ')':
			if start < i {
				fields = append(fields, filter[start:i])
			}
			start = i + 1
		}
	}
	if start < len(filter) {
		fields = append(fields, filter[start:])
	}
	return fields
}
