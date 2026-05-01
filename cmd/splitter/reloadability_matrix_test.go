package main

import "testing"

func TestWindowsReloadabilityMatrix_HasExpectedEntries(t *testing.T) {
	if len(windowsReloadableSettings) == 0 {
		t.Fatal("windowsReloadableSettings should not be empty")
	}
	if len(windowsRestartRequiredSettings) == 0 {
		t.Fatal("windowsRestartRequiredSettings should not be empty")
	}

	mustContain(t, windowsReloadableSettings, "engine.split_mode")
	mustContain(t, windowsReloadableSettings, "engine.policies")
	mustContain(t, windowsReloadableSettings, "engine.stats_interval")
	mustContain(t, windowsReloadableSettings, "windivert.filter (via handle reopen)")
	mustContain(t, windowsReloadableSettings, "windivert.queue_len (including 0/default via handle reopen)")
	mustContain(t, windowsRestartRequiredSettings, "windivert_dir")
}

func TestWindowsReloadabilityMatrix_NoOverlap(t *testing.T) {
	restartSet := make(map[string]struct{}, len(windowsRestartRequiredSettings))
	for _, s := range windowsRestartRequiredSettings {
		restartSet[s] = struct{}{}
	}
	for _, s := range windowsReloadableSettings {
		if _, ok := restartSet[s]; ok {
			t.Fatalf("setting appears in both lists: %q", s)
		}
	}
}

func mustContain(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("missing %q in %v", want, items)
}
