package adapter

import (
	"bytes"
	"sync"
	"testing"
)

func TestCopyIntoPoolBufferDoesNotReuseJumboBufferForMTUPacket(t *testing.T) {
	pool := sync.Pool{New: newPooledPacketBuffer}
	jumbo := make([]byte, 0xffff)
	pool.Put(&jumbo)

	_, backing := copyIntoPoolBuffer(&pool, bytes.Repeat([]byte{0xab}, 1500))
	if cap(backing) >= cap(jumbo) {
		t.Fatalf("small packet retained jumbo backing: %d bytes", cap(backing))
	}
}

func BenchmarkCopyIntoPoolBuffer(b *testing.B) {
	pool := sync.Pool{New: newPooledPacketBuffer}
	src := bytes.Repeat([]byte{0xab}, 1500)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data, backing := copyIntoPoolBuffer(&pool, src)
		if len(data) != len(src) {
			b.Fatalf("unexpected payload length: %d", len(data))
		}
		backing = backing[:cap(backing)]
		pool.Put(&backing)
	}
}

func BenchmarkAllocCopyBuffer(b *testing.B) {
	src := bytes.Repeat([]byte{0xab}, 1500)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data := append([]byte(nil), src...)
		if len(data) != len(src) {
			b.Fatalf("unexpected payload length: %d", len(data))
		}
	}
}
