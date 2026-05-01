package main

import "testing"

func TestParseEnginePolicies(t *testing.T) {
	splitChunk := 7
	policies, err := parseEnginePolicies([]enginePolicyJSONConfig{
		{
			Name:       "google",
			DstCIDRs:   []string{"8.8.8.0/24"},
			SplitChunk: &splitChunk,
		},
	})
	if err != nil {
		t.Fatalf("parseEnginePolicies failed: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
	if policies[0].Name != "google" {
		t.Fatalf("unexpected policy name: %q", policies[0].Name)
	}
	if len(policies[0].DstPrefixes) != 1 {
		t.Fatalf("expected 1 prefix, got %d", len(policies[0].DstPrefixes))
	}
	if policies[0].Skip {
		t.Fatal("did not expect skip policy")
	}
	if !policies[0].HasSplitChunk || policies[0].SplitChunk != splitChunk {
		t.Fatalf("unexpected split_chunk: %+v", policies[0])
	}
}

func TestParseEnginePolicies_InvalidCIDR(t *testing.T) {
	_, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		DstCIDRs: []string{"not-a-cidr"},
		Skip:     boolPtr(true),
	}})
	if err == nil {
		t.Fatal("expected CIDR parse error")
	}
}

func TestParseEnginePolicies_RejectsNoOpPolicy(t *testing.T) {
	_, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		DstCIDRs: []string{"1.1.1.0/24"},
	}})
	if err == nil {
		t.Fatal("expected no-op policy error")
	}
}

func TestParseEnginePolicies_RejectsSkipWithSplitOverrides(t *testing.T) {
	_, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		DstCIDRs:   []string{"1.1.1.0/24"},
		Skip:       boolPtr(true),
		SplitChunk: intPtr(7),
	}})
	if err == nil {
		t.Fatal("expected skip/split override validation error")
	}
}

func TestParseEnginePolicies_SNISuffixes(t *testing.T) {
	policies, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		SNISuffixes: []string{".WWW.Example.COM."},
		Skip:        boolPtr(true),
	}})
	if err != nil {
		t.Fatalf("parseEnginePolicies failed: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
	if len(policies[0].SNISuffixes) != 1 || policies[0].SNISuffixes[0] != "www.example.com" {
		t.Fatalf("unexpected sni suffixes: %+v", policies[0].SNISuffixes)
	}
}

func TestParseEnginePolicies_SNIRejectsImmediateSplitMode(t *testing.T) {
	mode := "immediate"
	_, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		SNISuffixes: []string{"example.com"},
		SplitMode:   &mode,
		SplitChunk:  intPtr(9),
	}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseEnginePolicies_RejectsInvalidSNISuffix(t *testing.T) {
	_, err := parseEnginePolicies([]enginePolicyJSONConfig{{
		SNISuffixes: []string{"bad_suffix.example"},
		Skip:        boolPtr(true),
	}})
	if err == nil {
		t.Fatal("expected invalid SNI suffix error")
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}
