package packet

import (
	"sync"
	"testing"
)

func TestPacketReleasePreservesVisibleFields(t *testing.T) {
	pool := sync.Pool{
		New: func() any { return make([]byte, 16) },
	}
	backing, _ := pool.Get().([]byte)
	if cap(backing) < 8 {
		t.Fatalf("unexpected backing cap: %d", cap(backing))
	}
	data := backing[:8]
	copy(data, []byte{1, 2, 3, 4, 5, 6, 7, 8})

	pkt := &Packet{
		Data:   data,
		Addr:   Address{Data: [256]byte{1}},
		Meta:   Meta{PayloadOffset: 4, IPVersion: IPVersion4},
		Source: SourceCaptured,
		NFQID:  42,
	}
	pkt.SetDataPool(&pool, backing)

	pkt.Release()

	if len(pkt.Data) != 8 {
		t.Fatalf("Release should preserve visible data length, got %d", len(pkt.Data))
	}
	if got := pkt.Payload(); len(got) != 4 {
		t.Fatalf("Release should preserve payload view, got len=%d", len(got))
	}
	if pkt.Source != SourceCaptured {
		t.Fatalf("Release should preserve Source, got %v", pkt.Source)
	}
	if pkt.NFQID != 42 {
		t.Fatalf("Release should preserve NFQID, got %d", pkt.NFQID)
	}
	if pkt.dataPool != nil || pkt.dataBacking != nil {
		t.Fatal("Release should clear pool ownership")
	}

	// A second release must be a no-op.
	pkt.Release()
}
