package tls

import "testing"

func TestDetectClientHello_Valid(t *testing.T) {
	buf := []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01}
	recordLen, result := DetectClientHelloRecord(buf)
	if result != ResultMatch {
		t.Fatalf("expected Match, got %v", result)
	}
	if recordLen != 5 {
		t.Fatalf("expected recordLen=5, got %d", recordLen)
	}
}

func TestDetectClientHello_NeedMore(t *testing.T) {
	buf := []byte{0x16, 0x03, 0x01}
	_, result := DetectClientHelloRecord(buf)
	if result != ResultNeedMore {
		t.Fatalf("expected NeedMore, got %v", result)
	}
}

func TestDetectClientHello_WrongContentType(t *testing.T) {
	buf := []byte{0x15, 0x03, 0x01, 0x00, 0x05, 0x01}
	_, result := DetectClientHelloRecord(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func TestDetectClientHello_WrongVersion(t *testing.T) {
	buf := []byte{0x16, 0x02, 0x00, 0x00, 0x05, 0x01}
	_, result := DetectClientHelloRecord(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func TestDetectClientHello_WrongHandshakeType(t *testing.T) {
	buf := []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x02}
	_, result := DetectClientHelloRecord(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func TestDetectClientHello_ZeroRecordLen(t *testing.T) {
	buf := []byte{0x16, 0x03, 0x01, 0x00, 0x00, 0x01}
	_, result := DetectClientHelloRecord(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch for zero recordLen, got %v", result)
	}
}
