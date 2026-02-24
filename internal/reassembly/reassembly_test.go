package reassembly

import "testing"

func TestInOrderContiguous(t *testing.T) {
	buf := New(1000, 64)
	if err := buf.Push(1000, []byte("abc")); err != nil {
		t.Fatalf("push 1: %v", err)
	}
	if got := string(buf.Contiguous()); got != "abc" {
		t.Fatalf("contig 1 = %q", got)
	}
	if err := buf.Push(1003, []byte("def")); err != nil {
		t.Fatalf("push 2: %v", err)
	}
	if got := string(buf.Contiguous()); got != "abcdef" {
		t.Fatalf("contig 2 = %q", got)
	}
	if buf.HadOutOfOrder() || buf.HadOverlap() {
		t.Fatalf("unexpected flags: ooo=%v overlap=%v", buf.HadOutOfOrder(), buf.HadOverlap())
	}
}

func TestGapThenFill(t *testing.T) {
	buf := New(1000, 64)
	if err := buf.Push(1005, []byte("FGH")); err != nil {
		t.Fatalf("push gap: %v", err)
	}
	if got := string(buf.Contiguous()); got != "" {
		t.Fatalf("contig gap = %q", got)
	}
	if err := buf.Push(1000, []byte("abcde")); err != nil {
		t.Fatalf("push fill: %v", err)
	}
	if got := string(buf.Contiguous()); got != "abcdeFGH" {
		t.Fatalf("contig filled = %q", got)
	}
	if !buf.HadOutOfOrder() {
		t.Fatalf("expected out-of-order flag")
	}
}

func TestOverlapExtends(t *testing.T) {
	buf := New(1000, 64)
	if err := buf.Push(1000, []byte("abcdef")); err != nil {
		t.Fatalf("push 1: %v", err)
	}
	if err := buf.Push(1003, []byte("DEFGH")); err != nil {
		t.Fatalf("push overlap: %v", err)
	}
	if got := string(buf.Contiguous()); got != "abcdefGH" {
		t.Fatalf("contig overlap = %q", got)
	}
	if !buf.HadOverlap() {
		t.Fatalf("expected overlap flag")
	}
}

func TestOutOfWindow(t *testing.T) {
	buf := New(1000, 8)
	if err := buf.Push(1010, []byte("x")); err != ErrOutOfWindow {
		t.Fatalf("expected ErrOutOfWindow, got %v", err)
	}
}

func TestBufferFull(t *testing.T) {
	buf := New(1000, 5)
	if err := buf.Push(1000, []byte("abcd")); err != nil {
		t.Fatalf("push 1: %v", err)
	}
	if err := buf.Push(1004, []byte("ef")); err != ErrBufferFull {
		t.Fatalf("expected ErrBufferFull, got %v", err)
	}
	if got := string(buf.Contiguous()); got != "abcd" {
		t.Fatalf("contig after full = %q", got)
	}
}

func TestWrapAround(t *testing.T) {
	base := uint32(0xFFFFFFF0)
	buf := New(base, 64)
	if err := buf.Push(base, []byte("0123456789abcdef")); err != nil {
		t.Fatalf("push base: %v", err)
	}
	if err := buf.Push(0x00000000, []byte("Z")); err != nil {
		t.Fatalf("push wrap: %v", err)
	}
	if got := string(buf.Contiguous()); got != "0123456789abcdefZ" {
		t.Fatalf("contig wrap = %q", got)
	}
}

func TestPushOverflow(t *testing.T) {
	// maxBytes is large enough that offset passes the first check, but
	// offset + len(payload) would overflow uint32.
	base := uint32(0)
	maxBytes := uint32(0xFFFFFFF0)
	buf := New(base, maxBytes)

	// offset = 0xFFFFFFE0, len(payload) = 32 → end wraps to 0
	offset := uint32(0xFFFFFFE0)
	seq := base + offset
	payload := make([]byte, 32)
	err := buf.Push(seq, payload)
	if err != ErrBufferFull {
		t.Fatalf("expected ErrBufferFull on overflow, got %v", err)
	}
}
