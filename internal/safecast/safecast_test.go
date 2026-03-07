package safecast

import "testing"

func TestIntToUint16(t *testing.T) {
	if got, ok := IntToUint16(65535); !ok || got != 65535 {
		t.Fatalf("IntToUint16(65535) = (%d, %v)", got, ok)
	}
	if _, ok := IntToUint16(-1); ok {
		t.Fatal("IntToUint16(-1) unexpectedly succeeded")
	}
	if _, ok := IntToUint16(65536); ok {
		t.Fatal("IntToUint16(65536) unexpectedly succeeded")
	}
}

func TestIntToUint32(t *testing.T) {
	if got, ok := IntToUint32(65536); !ok || got != 65536 {
		t.Fatalf("IntToUint32(65536) = (%d, %v)", got, ok)
	}
	if _, ok := IntToUint32(-1); ok {
		t.Fatal("IntToUint32(-1) unexpectedly succeeded")
	}
}

func TestUint32ToUint16(t *testing.T) {
	if got, ok := Uint32ToUint16(65535); !ok || got != 65535 {
		t.Fatalf("Uint32ToUint16(65535) = (%d, %v)", got, ok)
	}
	if _, ok := Uint32ToUint16(65536); ok {
		t.Fatal("Uint32ToUint16(65536) unexpectedly succeeded")
	}
}

func TestUint64ToInt(t *testing.T) {
	if got, ok := Uint64ToInt(7); !ok || got != 7 {
		t.Fatalf("Uint64ToInt(7) = (%d, %v)", got, ok)
	}
}
