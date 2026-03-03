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
	mustContain(t, windowsReloadableSettings, "windivert.queue_len (non-zero values)")
	mustContain(t, windowsRestartRequiredSettings, "windivert.filter")
	mustContain(t, windowsRestartRequiredSettings, "windivert.queue_len=0 (revert to driver default)")
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

