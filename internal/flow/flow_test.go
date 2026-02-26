package flow

import (
	"testing"
	"time"

	"fk-gov/internal/packet"
)

func TestKeyFromMeta(t *testing.T) {
	meta := packet.Meta{
		SrcIP:   [4]byte{10, 0, 0, 2},
		DstIP:   [4]byte{1, 1, 1, 1},
		SrcPort: 54321,
		DstPort: 443,
		Proto:   6,
	}
	key := KeyFromMeta(meta)
	if key.SrcIP != meta.SrcIP || key.DstIP != meta.DstIP {
		t.Fatalf("ip mismatch: got %+v", key)
	}
	if key.SrcPort != meta.SrcPort || key.DstPort != meta.DstPort || key.Proto != meta.Proto {
		t.Fatalf("tuple mismatch: got %+v", key)
	}
}

func TestTableGetOrCreateAndDelete(t *testing.T) {
	tbl := NewTable()
	now := time.Now()
	key := Key{SrcIP: [4]byte{1, 2, 3, 4}, DstIP: [4]byte{8, 8, 8, 8}, SrcPort: 1234, DstPort: 443, Proto: 6}

	st := tbl.GetOrCreate(key, now)
	if st.State != StateNew {
		t.Fatalf("initial state: got %v", st.State)
	}
	if !st.LastActive.Equal(now) {
		t.Fatalf("last active mismatch: got %v want %v", st.LastActive, now)
	}
	if tbl.Len() != 1 {
		t.Fatalf("table len mismatch: got %d", tbl.Len())
	}

	later := now.Add(time.Minute)
	st2 := tbl.GetOrCreate(key, later)
	if st2 != st {
		t.Fatalf("expected same state pointer for existing key")
	}
	if !st2.LastActive.Equal(now) {
		t.Fatalf("GetOrCreate must not overwrite existing LastActive")
	}

	if got, ok := tbl.Get(key); !ok || got != st {
		t.Fatalf("Get did not return stored state")
	}

	tbl.Delete(key)
	if _, ok := tbl.Get(key); ok {
		t.Fatalf("expected key to be deleted")
	}
	if tbl.Len() != 0 {
		t.Fatalf("table len after delete: got %d", tbl.Len())
	}
}

func TestTableRange(t *testing.T) {
	tbl := NewTable()
	now := time.Now()
	k1 := Key{SrcIP: [4]byte{10, 0, 0, 1}, DstIP: [4]byte{1, 1, 1, 1}, SrcPort: 1000, DstPort: 443, Proto: 6}
	k2 := Key{SrcIP: [4]byte{10, 0, 0, 2}, DstIP: [4]byte{1, 1, 1, 1}, SrcPort: 1001, DstPort: 443, Proto: 6}
	tbl.GetOrCreate(k1, now)
	tbl.GetOrCreate(k2, now)

	seen := map[Key]bool{}
	tbl.Range(func(k Key, st *FlowState) {
		if st == nil {
			t.Fatalf("state must not be nil")
		}
		seen[k] = true
	})

	if len(seen) != 2 || !seen[k1] || !seen[k2] {
		t.Fatalf("range did not visit all keys: %+v", seen)
	}
}

func TestSharderIndex(t *testing.T) {
	s := NewSharder(8)
	key := Key{SrcIP: [4]byte{10, 1, 1, 1}, DstIP: [4]byte{8, 8, 8, 8}, SrcPort: 2345, DstPort: 443, Proto: 6}

	i1 := s.Index(key)
	i2 := s.Index(key)
	if i1 != i2 {
		t.Fatalf("same key must hash to same index: %d vs %d", i1, i2)
	}
	if i1 < 0 || i1 >= s.Workers() {
		t.Fatalf("index out of bounds: %d workers=%d", i1, s.Workers())
	}
}

func TestNewSharderMinimumWorkers(t *testing.T) {
	s := NewSharder(0)
	if s.Workers() != 1 {
		t.Fatalf("workers must clamp to 1, got %d", s.Workers())
	}
}
