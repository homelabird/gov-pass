package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"fk-gov/internal/engine"
)

const maxJSONConfigFileBytes = 1 << 20

func readJSONConfigFile(path string) ([]byte, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("config path must not be empty")
	}

	f, info, err := openRegularConfigFile(clean)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	if info.Size() > maxJSONConfigFileBytes {
		return nil, fmt.Errorf("config file exceeds %d bytes: %s", maxJSONConfigFileBytes, clean)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxJSONConfigFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxJSONConfigFileBytes {
		return nil, fmt.Errorf("config file exceeds %d bytes: %s", maxJSONConfigFileBytes, clean)
	}
	return b, nil
}

func decodeStrictJSONConfig(data []byte, dst any) error {
	if err := rejectDuplicateJSONFields(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := scanJSONValueForDuplicateFields(dec, "$"); err != nil {
		return err
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected trailing JSON value after %v", tok)
	}
	return nil
}

func scanJSONValueForDuplicateFields(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return fmt.Errorf("%s: expected object key, got %T", path, keyTok)
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("%s: duplicate field %q", path, key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValueForDuplicateFields(dec, jsonChildPath(path, key)); err != nil {
				return err
			}
		}
		endTok, err := dec.Token()
		if err != nil {
			return err
		}
		if endTok != json.Delim('}') {
			return fmt.Errorf("%s: expected object end, got %v", path, endTok)
		}
	case '[':
		for i := 0; dec.More(); i++ {
			if err := scanJSONValueForDuplicateFields(dec, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		endTok, err := dec.Token()
		if err != nil {
			return err
		}
		if endTok != json.Delim(']') {
			return fmt.Errorf("%s: expected array end, got %v", path, endTok)
		}
	default:
		return fmt.Errorf("%s: unexpected JSON delimiter %q", path, delim)
	}
	return nil
}

func jsonChildPath(path string, key string) string {
	if path == "" {
		return key
	}
	if key == "" {
		return path + `[""]`
	}
	if strings.ContainsAny(key, ".[]") {
		return fmt.Sprintf("%s[%q]", path, key)
	}
	return path + "." + key
}

type engineJSONConfig struct {
	SplitMode                   *string                  `json:"split_mode,omitempty"`
	SplitChunk                  *int                     `json:"split_chunk,omitempty"`
	CollectTimeout              *string                  `json:"collect_timeout,omitempty"`
	MaxBufferBytes              *int                     `json:"max_buffer_bytes,omitempty"`
	MaxHeldPackets              *int                     `json:"max_held_packets,omitempty"`
	MaxSegmentPayload           *int                     `json:"max_segment_payload,omitempty"`
	Workers                     *int                     `json:"workers,omitempty"`
	FlowIdleTimeout             *string                  `json:"flow_idle_timeout,omitempty"`
	GCInterval                  *string                  `json:"gc_interval,omitempty"`
	MaxFlowsPerWorker           *int                     `json:"max_flows_per_worker,omitempty"`
	MaxReassemblyBytesPerWorker *int                     `json:"max_reassembly_bytes_per_worker,omitempty"`
	MaxHeldBytesPerWorker       *int                     `json:"max_held_bytes_per_worker,omitempty"`
	ShutdownFailOpenTimeout     *string                  `json:"shutdown_fail_open_timeout,omitempty"`
	ShutdownFailOpenMaxPackets  *int                     `json:"shutdown_fail_open_max_packets,omitempty"`
	AdapterFlushTimeout         *string                  `json:"adapter_flush_timeout,omitempty"`
	StatsInterval               *string                  `json:"stats_interval,omitempty"`
	Policies                    []enginePolicyJSONConfig `json:"policies,omitempty"`
}

type engineFlagRefs struct {
	SplitMode                  *string
	SplitChunk                 *int
	CollectTimeout             *time.Duration
	MaxBuffer                  *int
	MaxHeld                    *int
	MaxSegPayload              *int
	Workers                    *int
	FlowTimeout                *time.Duration
	GCInterval                 *time.Duration
	MaxFlows                   *int
	MaxReassembly              *int
	MaxHeldBytes               *int
	ShutdownFailOpenTimeout    *time.Duration
	ShutdownFailOpenMaxPackets *int
	AdapterFlushTimeout        *time.Duration
	StatsInterval              *time.Duration
	Policies                   *[]engine.Policy
}

func applyEngineJSONConfigToFlags(cfg *engineJSONConfig, setFlags map[string]bool, refs engineFlagRefs) error {
	if cfg == nil {
		return nil
	}
	if cfg.SplitMode != nil && !setFlags["split-mode"] {
		v := strings.TrimSpace(*cfg.SplitMode)
		if v != "" && refs.SplitMode != nil {
			*refs.SplitMode = v
		}
	}
	if cfg.SplitChunk != nil && !setFlags["split-chunk"] && refs.SplitChunk != nil {
		*refs.SplitChunk = *cfg.SplitChunk
	}
	if cfg.CollectTimeout != nil && !setFlags["collect-timeout"] {
		d, err := parseOptionalDuration("engine.collect_timeout", *cfg.CollectTimeout)
		if err != nil {
			return err
		}
		if d != nil && refs.CollectTimeout != nil {
			*refs.CollectTimeout = *d
		}
	}
	if cfg.MaxBufferBytes != nil && !setFlags["max-buffer"] && refs.MaxBuffer != nil {
		*refs.MaxBuffer = *cfg.MaxBufferBytes
	}
	if cfg.MaxHeldPackets != nil && !setFlags["max-held-pkts"] && refs.MaxHeld != nil {
		*refs.MaxHeld = *cfg.MaxHeldPackets
	}
	if cfg.MaxSegmentPayload != nil && !setFlags["max-seg-payload"] && refs.MaxSegPayload != nil {
		*refs.MaxSegPayload = *cfg.MaxSegmentPayload
	}
	if cfg.Workers != nil && !setFlags["workers"] && refs.Workers != nil {
		*refs.Workers = *cfg.Workers
	}
	if cfg.FlowIdleTimeout != nil && !setFlags["flow-timeout"] {
		d, err := parseOptionalDuration("engine.flow_idle_timeout", *cfg.FlowIdleTimeout)
		if err != nil {
			return err
		}
		if d != nil && refs.FlowTimeout != nil {
			*refs.FlowTimeout = *d
		}
	}
	if cfg.GCInterval != nil && !setFlags["gc-interval"] {
		d, err := parseOptionalDuration("engine.gc_interval", *cfg.GCInterval)
		if err != nil {
			return err
		}
		if d != nil && refs.GCInterval != nil {
			*refs.GCInterval = *d
		}
	}
	if cfg.MaxFlowsPerWorker != nil && !setFlags["max-flows-per-worker"] && refs.MaxFlows != nil {
		*refs.MaxFlows = *cfg.MaxFlowsPerWorker
	}
	if cfg.MaxReassemblyBytesPerWorker != nil && !setFlags["max-reassembly-bytes-per-worker"] && refs.MaxReassembly != nil {
		*refs.MaxReassembly = *cfg.MaxReassemblyBytesPerWorker
	}
	if cfg.MaxHeldBytesPerWorker != nil && !setFlags["max-held-bytes-per-worker"] && refs.MaxHeldBytes != nil {
		*refs.MaxHeldBytes = *cfg.MaxHeldBytesPerWorker
	}
	if cfg.ShutdownFailOpenTimeout != nil && !setFlags["shutdown-fail-open-timeout"] {
		d, err := parseOptionalDuration("engine.shutdown_fail_open_timeout", *cfg.ShutdownFailOpenTimeout)
		if err != nil {
			return err
		}
		if d != nil && refs.ShutdownFailOpenTimeout != nil {
			*refs.ShutdownFailOpenTimeout = *d
		}
	}
	if cfg.ShutdownFailOpenMaxPackets != nil && !setFlags["shutdown-fail-open-max-pkts"] && refs.ShutdownFailOpenMaxPackets != nil {
		*refs.ShutdownFailOpenMaxPackets = *cfg.ShutdownFailOpenMaxPackets
	}
	if cfg.AdapterFlushTimeout != nil && !setFlags["adapter-flush-timeout"] {
		d, err := parseOptionalDuration("engine.adapter_flush_timeout", *cfg.AdapterFlushTimeout)
		if err != nil {
			return err
		}
		if d != nil && refs.AdapterFlushTimeout != nil {
			*refs.AdapterFlushTimeout = *d
		}
	}
	if cfg.StatsInterval != nil && !setFlags["stats-interval"] {
		d, err := parseOptionalDuration("engine.stats_interval", *cfg.StatsInterval)
		if err != nil {
			return err
		}
		if d != nil && refs.StatsInterval != nil {
			*refs.StatsInterval = *d
		}
	}
	if cfg.Policies != nil && refs.Policies != nil {
		policies, err := parseEnginePolicies(cfg.Policies)
		if err != nil {
			return err
		}
		*refs.Policies = policies
	}
	return nil
}

func parseOptionalDuration(label string, value string) (*time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return &d, nil
}
