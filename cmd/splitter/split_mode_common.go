package main

import (
	"errors"
	"strings"

	"fk-gov/internal/engine"
)

func parseSplitMode(value string) (engine.SplitMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "immediate":
		return engine.SplitModeImmediate, nil
	case "tls-hello":
		return engine.SplitModeTLSHello, nil
	default:
		return engine.SplitModeTLSHello, errors.New("expected tls-hello or immediate")
	}
}
