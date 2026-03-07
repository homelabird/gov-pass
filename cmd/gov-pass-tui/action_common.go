package main

import (
	"errors"
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

func normalizeServiceName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", errors.New("service name is empty")
	}
	if len(normalized) > 128 {
		return "", errors.New("service name is too long")
	}
	if strings.HasPrefix(normalized, "-") {
		return "", errors.New("service name cannot start with '-'")
	}
	for i := 0; i < len(normalized); i++ {
		c := normalized[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == '@':
		default:
			return "", fmt.Errorf("service name contains unsupported character %q", c)
		}
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
