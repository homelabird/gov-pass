package main

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var allowedTUIActions = map[string]struct{}{
	"start":   {},
	"stop":    {},
	"restart": {},
	"reload":  {},
	"enable":  {},
	"disable": {},
	"toggle":  {},
	"status":  {},
}

func normalizeAction(action string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(action))
	if _, ok := allowedTUIActions[normalized]; !ok {
		return "", fmt.Errorf("unknown action: %s", normalized)
	}
	return normalized, nil
}

type actionGate struct {
	busy uint32
}

func (g *actionGate) tryBegin() bool {
	return atomic.CompareAndSwapUint32(&g.busy, 0, 1)
}

func (g *actionGate) end() {
	atomic.StoreUint32(&g.busy, 0)
}
