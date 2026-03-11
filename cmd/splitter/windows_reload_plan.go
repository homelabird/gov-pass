package main

import "fk-gov/internal/adapter"

func requiresWinDivertReopenForReload(curFilter string, cur adapter.WinDivertOptions, nextFilter string, next adapter.WinDivertOptions) bool {
	if nextFilter != curFilter {
		return true
	}
	return queueResetRequiresReopen(cur, next)
}

func queueResetRequiresReopen(cur adapter.WinDivertOptions, next adapter.WinDivertOptions) bool {
	return (next.QueueLen == 0 && next.QueueLen != cur.QueueLen) ||
		(next.QueueTime == 0 && next.QueueTime != cur.QueueTime) ||
		(next.QueueSize == 0 && next.QueueSize != cur.QueueSize)
}
