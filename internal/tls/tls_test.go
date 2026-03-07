package tls

import (
	"encoding/binary"
	"testing"
)

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

func TestParseClientHello_ServerName(t *testing.T) {
	buf := buildClientHelloRecord("WWW.Example.COM")
	info, result := ParseClientHello(buf)
	if result != ResultMatch {
		t.Fatalf("expected Match, got %v", result)
	}
	if info.ServerName != "www.example.com" {
		t.Fatalf("unexpected server name: %q", info.ServerName)
	}
}

func TestParseClientHello_NoServerName(t *testing.T) {
	buf := buildClientHelloRecord("")
	info, result := ParseClientHello(buf)
	if result != ResultMatch {
		t.Fatalf("expected Match, got %v", result)
	}
	if info.ServerName != "" {
		t.Fatalf("expected empty server name, got %q", info.ServerName)
	}
}

func TestParseClientHello_NeedMore(t *testing.T) {
	full := buildClientHelloRecord("example.com")
	info, result := ParseClientHello(full[:len(full)-1])
	if result != ResultNeedMore {
		t.Fatalf("expected NeedMore, got %v info=%+v", result, info)
	}
}

func buildClientHelloRecord(serverName string) []byte {
	extensions := []byte{}
	if serverName != "" {
		name := []byte(serverName)
		serverNameData := make([]byte, 2+1+2+len(name))
		binary.BigEndian.PutUint16(serverNameData[0:2], uint16(3+len(name)))
		serverNameData[2] = 0
		binary.BigEndian.PutUint16(serverNameData[3:5], uint16(len(name)))
		copy(serverNameData[5:], name)

		ext := make([]byte, 4+len(serverNameData))
		binary.BigEndian.PutUint16(ext[0:2], 0)
		binary.BigEndian.PutUint16(ext[2:4], uint16(len(serverNameData)))
		copy(ext[4:], serverNameData)
		extensions = append(extensions, ext...)
	}

	body := make([]byte, 0, 64+len(extensions))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0x00)
	body = append(body, 0x00, 0x02, 0x13, 0x01)
	body = append(body, 0x01, 0x00)
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)

	handshake := make([]byte, 4+len(body))
	handshake[0] = 0x01
	handshake[1] = byte(len(body) >> 16)
	handshake[2] = byte(len(body) >> 8)
	handshake[3] = byte(len(body))
	copy(handshake[4:], body)

	record := make([]byte, 5+len(handshake))
	record[0] = 0x16
	record[1] = 0x03
	record[2] = 0x03
	binary.BigEndian.PutUint16(record[3:5], uint16(len(handshake)))
	copy(record[5:], handshake)
	return record
}
