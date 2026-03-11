package engine

import (
	"net/netip"
	"testing"

	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
	itls "fk-gov/internal/tls"
)

func TestConfigPlanForMeta_FirstMatchWins(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 5
	cfg.Policies = []Policy{
		{
			Name:          "broad",
			DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("8.8.8.0/24")},
			HasSplitChunk: true,
			SplitChunk:    9,
		},
		{
			Name:          "narrow",
			DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("8.8.8.8/32")},
			HasSplitChunk: true,
			SplitChunk:    11,
		},
	}

	plan, resolved := cfg.resolvePlan(meta4(8, 8, 8, 8), nil)
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if plan.SplitChunk != 9 {
		t.Fatalf("expected first matching policy to win, got split_chunk=%d", plan.SplitChunk)
	}
}

func TestConfigPlanForMeta_SkipPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policies = []Policy{{
		DstPrefixes: []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24")},
		Skip:        true,
	}}

	plan, resolved := cfg.resolvePlan(meta4(1, 1, 1, 1), nil)
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if !plan.Skip {
		t.Fatal("expected skip policy")
	}
}

func TestConfigResolvePlan_SNIRequiresHello(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policies = []Policy{{
		SNISuffixes:   []string{"example.com"},
		HasSplitChunk: true,
		SplitChunk:    9,
	}}

	_, resolved := cfg.resolvePlan(meta4(8, 8, 8, 8), nil)
	if resolved {
		t.Fatal("expected unresolved plan before hello is parsed")
	}

	plan, resolved := cfg.resolvePlan(meta4(8, 8, 8, 8), &itls.ClientHelloInfo{ServerName: "www.example.com"})
	if !resolved {
		t.Fatal("expected resolved plan after hello is parsed")
	}
	if plan.SplitChunk != 9 {
		t.Fatalf("unexpected split chunk: %d", plan.SplitChunk)
	}
}

func TestConfigResolvePlan_SNIFallsThroughAfterNoMatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 5
	cfg.Policies = []Policy{
		{
			SNISuffixes:   []string{"blocked.example"},
			HasSplitChunk: true,
			SplitChunk:    11,
		},
		{
			DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("8.8.8.0/24")},
			HasSplitChunk: true,
			SplitChunk:    7,
		},
	}

	plan, resolved := cfg.resolvePlan(meta4(8, 8, 8, 8), &itls.ClientHelloInfo{ServerName: "www.example.com"})
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if plan.SplitChunk != 7 {
		t.Fatalf("expected fallback ip policy split_chunk=7, got %d", plan.SplitChunk)
	}
}

func TestResolveFlowPlan_StickyAcrossReload(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Policies = []Policy{{
		DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("9.9.9.0/24")},
		HasSplitChunk: true,
		SplitChunk:    7,
	}}
	w := newWorker(0, cfg, nil, newStats())
	st := &flow.FlowState{}
	meta := meta4(9, 9, 9, 9)

	first, resolved := w.resolveFlowPlan(st, meta, nil, cfg)
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if first.SplitChunk != 7 {
		t.Fatalf("expected initial split_chunk=7, got %d", first.SplitChunk)
	}

	next := cfg
	next.Policies = []Policy{{
		DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("9.9.9.0/24")},
		HasSplitChunk: true,
		SplitChunk:    13,
	}}
	second, resolved := w.resolveFlowPlan(st, meta, nil, next)
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if second.SplitChunk != 7 {
		t.Fatalf("expected cached split_chunk=7, got %d", second.SplitChunk)
	}
}

func TestConfigPlanForMeta_IPv6Prefix(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 5
	cfg.Policies = []Policy{{
		DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")},
		HasSplitChunk: true,
		SplitChunk:    17,
	}}

	plan, resolved := cfg.resolvePlan(meta6("2001:db8::1234"), nil)
	if !resolved {
		t.Fatal("expected resolved plan")
	}
	if plan.SplitChunk != 17 {
		t.Fatalf("expected ipv6 policy split_chunk=17, got %d", plan.SplitChunk)
	}
}

func TestValidateConfig_RejectsSNIPolicyWithEffectiveImmediateMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = SplitModeImmediate
	cfg.Policies = []Policy{{
		SNISuffixes:   []string{"example.com"},
		HasSplitChunk: true,
		SplitChunk:    9,
	}}

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateConfig_AllowsSNIPolicyWhenPolicyOverridesToTLSHello(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = SplitModeImmediate
	cfg.Policies = []Policy{{
		SNISuffixes:   []string{"example.com"},
		HasSplitMode:  true,
		SplitMode:     SplitModeTLSHello,
		HasSplitChunk: true,
		SplitChunk:    9,
	}}

	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}
}

func TestValidateConfig_RejectsMaxBufferAboveUint32(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxBufferBytes = int(maxUint32Int64) + 1

	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected validation error")
	}
}

func meta4(a, b, c, d byte) packet.Meta {
	return packet.Meta{
		IPVersion: packet.IPVersion4,
		DstIP:     [16]byte{a, b, c, d},
	}
}

func meta6(addr string) packet.Meta {
	parsed := netip.MustParseAddr(addr)
	return packet.Meta{
		IPVersion: packet.IPVersion6,
		DstIP:     parsed.As16(),
	}
}
