package main

import (
	"testing"

	"fk-gov/internal/adapter"
)

func TestRequiresWinDivertReopenForReload(t *testing.T) {
	tests := []struct {
		name       string
		curFilter  string
		curOpts    adapter.WinDivertOptions
		nextFilter string
		nextOpts   adapter.WinDivertOptions
		wantReopen bool
	}{
		{
			name:       "no reopen for unchanged non-zero queue updates",
			curFilter:  "outbound and tcp.DstPort == 443",
			curOpts:    adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			nextFilter: "outbound and tcp.DstPort == 443",
			nextOpts:   adapter.WinDivertOptions{QueueLen: 8192, QueueTime: 2048, QueueSize: 4194304},
		},
		{
			name:       "filter change triggers reopen",
			curFilter:  "outbound and tcp.DstPort == 443",
			curOpts:    adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			nextFilter: "outbound and tcp.DstPort == 8443",
			nextOpts:   adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			wantReopen: true,
		},
		{
			name:       "queue len reset triggers reopen",
			curFilter:  "outbound and tcp.DstPort == 443",
			curOpts:    adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			nextFilter: "outbound and tcp.DstPort == 443",
			nextOpts:   adapter.WinDivertOptions{QueueLen: 0, QueueTime: 2048, QueueSize: 4194304},
			wantReopen: true,
		},
		{
			name:       "queue time reset triggers reopen",
			curFilter:  "outbound and tcp.DstPort == 443",
			curOpts:    adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			nextFilter: "outbound and tcp.DstPort == 443",
			nextOpts:   adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 0, QueueSize: 4194304},
			wantReopen: true,
		},
		{
			name:       "queue size reset triggers reopen",
			curFilter:  "outbound and tcp.DstPort == 443",
			curOpts:    adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 4194304},
			nextFilter: "outbound and tcp.DstPort == 443",
			nextOpts:   adapter.WinDivertOptions{QueueLen: 4096, QueueTime: 2048, QueueSize: 0},
			wantReopen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiresWinDivertReopenForReload(tt.curFilter, tt.curOpts, tt.nextFilter, tt.nextOpts)
			if got != tt.wantReopen {
				t.Fatalf("requiresWinDivertReopenForReload() = %v, want %v", got, tt.wantReopen)
			}
		})
	}
}
