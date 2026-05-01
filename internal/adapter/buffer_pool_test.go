package adapter

import (
	"bytes"
	"sync"
	"testing"
)

func TestCopyIntoPoolBufferCopiesData(t *testing.T) {
	pool := sync.Pool{
		New: func() any { return make([]byte, 16) },
	}
	src := []byte("hello")

	data, backing := copyIntoPoolBuffer(&pool, src)
	if !bytes.Equal(data, src) {
		t.Fatalf("copied payload mismatch: got=%q want=%q", data, src)
	}
	if &data[0] != &backing[0] {
		t.Fatal("expected data to reference pooled backing storage")
	}
}

func BenchmarkCopyIntoPoolBuffer(b *testing.B) {
	pool := sync.Pool{
		New: func() any { return make([]byte, 2048) },
	}
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
