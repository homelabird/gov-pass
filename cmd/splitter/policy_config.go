package main

import (
	"fmt"
	"net/netip"
	"strings"

	"fk-gov/internal/engine"
)

type enginePolicyJSONConfig struct {
	Name              string   `json:"name,omitempty"`
	DstCIDRs          []string `json:"dst_cidrs,omitempty"`
	SNISuffixes       []string `json:"sni_suffixes,omitempty"`
	Skip              *bool    `json:"skip,omitempty"`
	SplitMode         *string  `json:"split_mode,omitempty"`
	SplitChunk        *int     `json:"split_chunk,omitempty"`
	MaxSegmentPayload *int     `json:"max_segment_payload,omitempty"`
}

func parseEnginePolicies(items []enginePolicyJSONConfig) ([]engine.Policy, error) {
	if len(items) == 0 {
		return nil, nil
	}
	policies := make([]engine.Policy, 0, len(items))
	for i, item := range items {
		policy := engine.Policy{
			Name: strings.TrimSpace(item.Name),
		}
		if item.DstCIDRs != nil {
			for _, raw := range item.DstCIDRs {
				value := strings.TrimSpace(raw)
				if value == "" {
					return nil, fmt.Errorf("engine.policies[%d].dst_cidrs: empty value", i)
				}
				prefix, err := netip.ParsePrefix(value)
				if err != nil {
					return nil, fmt.Errorf("engine.policies[%d].dst_cidrs: %w", i, err)
				}
				policy.DstPrefixes = append(policy.DstPrefixes, prefix)
			}
		}
		for _, raw := range item.SNISuffixes {
			value := strings.TrimSpace(strings.ToLower(raw))
			value = strings.TrimPrefix(value, ".")
			value = strings.TrimSuffix(value, ".")
			if value == "" {
				return nil, fmt.Errorf("engine.policies[%d].sni_suffixes: empty value", i)
			}
			policy.SNISuffixes = append(policy.SNISuffixes, value)
		}
		if item.Skip != nil {
			policy.Skip = *item.Skip
		}
		if item.SplitMode != nil && strings.TrimSpace(*item.SplitMode) != "" {
			mode, err := parseSplitMode(*item.SplitMode)
			if err != nil {
				return nil, fmt.Errorf("engine.policies[%d].split_mode: %w", i, err)
			}
			policy.HasSplitMode = true
			policy.SplitMode = mode
		}
		if item.SplitChunk != nil {
			policy.HasSplitChunk = true
			policy.SplitChunk = *item.SplitChunk
		}
		if item.MaxSegmentPayload != nil {
			policy.HasMaxSegmentPayload = true
			policy.MaxSegmentPayload = *item.MaxSegmentPayload
		}
		if err := engine.ValidatePolicy(policy); err != nil {
			return nil, fmt.Errorf("engine.policies[%d]: %w", i, err)
		}
		policies = append(policies, policy)
	}
	return policies, nil
}
