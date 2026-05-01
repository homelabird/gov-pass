package tls

import (
	"encoding/binary"
	"strings"
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
	buf := buildClientHelloRecord("WWW.Example.COM.")
	info, result := ParseClientHello(buf)
	if result != ResultMatch {
		t.Fatalf("expected Match, got %v", result)
	}
	if info.ServerName != "www.example.com" {
		t.Fatalf("unexpected server name: %q", info.ServerName)
	}
}

func TestParseClientHello_RejectsInvalidServerName(t *testing.T) {
	longLabel := strings.Repeat("a", 64) + ".example"
	longName := strings.Repeat("a.", 127) + "a"
	tests := []string{
		" example.com",
		".example.com",
		"example..com",
		"-example.com",
		"example-.com",
		"bad_name.example",
		"*.example.com",
		"example.com/",
		"192.0.2.1",
		"2001:db8::1",
		"example\x00.com",
		"\xff.example.com",
		longLabel,
		longName,
	}

	for _, serverName := range tests {
		t.Run(serverName, func(t *testing.T) {
			_, result := ParseClientHello(buildClientHelloRecord(serverName))
			if result != ResultMismatch {
				t.Fatalf("expected Mismatch, got %v", result)
			}
		})
	}
}

func TestParseClientHello_RejectsMalformedServerNameExtension(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "empty list",
			body: []byte{0x00, 0x00},
		},
		{
			name: "unknown name type only",
			body: []byte{0x00, 0x03, 0x01, 0x00, 0x00},
		},
		{
			name: "duplicate host name",
			body: buildServerNameList("a.example", "b.example"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, result := ParseClientHello(buildClientHelloRecordWithServerNameExtension(tt.body))
			if result != ResultMismatch {
				t.Fatalf("expected Mismatch, got %v", result)
			}
		})
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

func TestParseClientHello_RejectsTrailingBytesAfterExtensions(t *testing.T) {
	buf := appendClientHelloBodyByte(buildClientHelloRecord("example.com"), 0)
	_, result := ParseClientHello(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func TestParseClientHello_RejectsTrailingBytesAfterHandshake(t *testing.T) {
	buf := appendTLSRecordByte(buildClientHelloRecord("example.com"), 0)
	_, result := ParseClientHello(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func TestParseClientHello_RejectsOddCipherSuiteLength(t *testing.T) {
	buf := buildClientHelloRecord("example.com")
	// Cipher suite length follows legacy version, random, and session ID length.
	cipherSuitesLenOffset := recordHeaderLen + handshakeHeaderLen + 2 + 32 + 1
	binary.BigEndian.PutUint16(buf[cipherSuitesLenOffset:cipherSuitesLenOffset+2], 1)
	_, result := ParseClientHello(buf)
	if result != ResultMismatch {
		t.Fatalf("expected Mismatch, got %v", result)
	}
}

func appendClientHelloBodyByte(record []byte, b byte) []byte {
	out := append([]byte(nil), record...)
	recordLen := int(binary.BigEndian.Uint16(out[3:5]))
	handshakeLen := int(out[6])<<16 | int(out[7])<<8 | int(out[8])
	out = append(out, b)
	recordLen++
	handshakeLen++
	binary.BigEndian.PutUint16(out[3:5], uint16(recordLen))
	out[6] = byte(handshakeLen >> 16)
	out[7] = byte(handshakeLen >> 8)
	out[8] = byte(handshakeLen)
	return out
}

func appendTLSRecordByte(record []byte, b byte) []byte {
	out := append([]byte(nil), record...)
	recordLen := int(binary.BigEndian.Uint16(out[3:5]))
	out = append(out, b)
	recordLen++
	binary.BigEndian.PutUint16(out[3:5], uint16(recordLen))
	return out
}

func buildClientHelloRecord(serverName string) []byte {
	extensions := []byte{}
	if serverName != "" {
		extensions = append(extensions, buildServerNameExtension(buildServerNameList(serverName))...)
	}
	return buildClientHelloRecordWithExtensions(extensions)
}

func buildClientHelloRecordWithServerNameExtension(body []byte) []byte {
	return buildClientHelloRecordWithExtensions(buildServerNameExtension(body))
}

func buildClientHelloRecordWithExtensions(extensions []byte) []byte {
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

func buildServerNameExtension(body []byte) []byte {
	ext := make([]byte, 4+len(body))
	binary.BigEndian.PutUint16(ext[0:2], 0)
	binary.BigEndian.PutUint16(ext[2:4], uint16(len(body)))
	copy(ext[4:], body)
	return ext
}

func buildServerNameList(names ...string) []byte {
	listLen := 0
	for _, name := range names {
		listLen += 3 + len(name)
	}
	out := make([]byte, 2, 2+listLen)
	binary.BigEndian.PutUint16(out[0:2], uint16(listLen))
	for _, name := range names {
		out = append(out, serverNameTypeHostName)
		out = append(out, byte(len(name)>>8), byte(len(name)))
		out = append(out, name...)
	}
	return out
}
