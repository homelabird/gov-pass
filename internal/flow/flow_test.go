package flow

import (
	"testing"
	"time"

	"fk-gov/internal/packet"
)

func TestTableGetOrCreateAndDelete(t *testing.T) {
	tbl := NewTable()
	now := time.Now()
	key := Key{IPVersion: packet.IPVersion4, SrcIP: v4Bytes(1, 2, 3, 4), DstIP: v4Bytes(8, 8, 8, 8), SrcPort: 1234, DstPort: 443, Proto: 6}

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

func TestSharderIndex(t *testing.T) {
	s := NewSharder(8)
	key := Key{IPVersion: packet.IPVersion4, SrcIP: v4Bytes(10, 1, 1, 1), DstIP: v4Bytes(8, 8, 8, 8), SrcPort: 2345, DstPort: 443, Proto: 6}

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

func v4Bytes(a, b, c, d byte) [16]byte {
	return [16]byte{a, b, c, d}
}
