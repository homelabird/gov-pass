package main

import "strings"

const (
	legacyWinDivertFilter  = "outbound and ip and tcp.DstPort == 443"
	defaultWinDivertFilter = "outbound and (ip or ipv6) and tcp.DstPort == 443"
)

func canonicalizeWinDivertFilter(value string) string {
	fields := strings.Fields(strings.ToLower(value))
	return strings.Join(fields, " ")
}

func isLegacyWinDivertFilter(value string) bool {
	return canonicalizeWinDivertFilter(value) == canonicalizeWinDivertFilter(legacyWinDivertFilter)
}

func upgradeLegacyWinDivertFilter(value string) string {
	if isLegacyWinDivertFilter(value) {
		return defaultWinDivertFilter
	}
	return value
}
