package tls

import (
	"encoding/binary"
	"strings"
)

type Result uint8

const (
	ResultNeedMore Result = iota
	ResultMismatch
	ResultMatch
)

const (
	recordHeaderLen         = 5
	handshakeHeaderLen      = 4
	clientHelloHeaderLen    = 2 + 32 + 1
	extensionTypeServerName = 0
	serverNameTypeHostName  = 0
)

type ClientHelloInfo struct {
	RecordLen  uint16
	ServerName string
}

// DetectClientHelloRecord checks for a TLS ClientHello in a contiguous buffer.
// It returns Match when contentType, version, and handshake type are valid.
func DetectClientHelloRecord(buf []byte) (uint16, Result) {
	if len(buf) < 6 {
		return 0, ResultNeedMore
	}
	if buf[0] != 0x16 {
		return 0, ResultMismatch
	}
	ver := uint16(buf[1])<<8 | uint16(buf[2])
	if ver < 0x0301 || ver > 0x0304 {
		return 0, ResultMismatch
	}
	if buf[5] != 0x01 {
		return 0, ResultMismatch
	}
	recordLen := uint16(buf[3])<<8 | uint16(buf[4])
	if recordLen == 0 {
		return 0, ResultMismatch
	}
	return recordLen, ResultMatch
}

func ParseClientHello(buf []byte) (ClientHelloInfo, Result) {
	recordLen, result := DetectClientHelloRecord(buf)
	if result != ResultMatch {
		return ClientHelloInfo{}, result
	}
	need := recordHeaderLen + int(recordLen)
	if len(buf) < need {
		return ClientHelloInfo{}, ResultNeedMore
	}
	if recordLen < handshakeHeaderLen {
		return ClientHelloInfo{}, ResultMismatch
	}

	handshakeLen := int(buf[6])<<16 | int(buf[7])<<8 | int(buf[8])
	if handshakeLen <= 0 || handshakeHeaderLen+handshakeLen > int(recordLen) {
		return ClientHelloInfo{}, ResultMismatch
	}

	body := buf[recordHeaderLen+handshakeHeaderLen : recordHeaderLen+handshakeHeaderLen+handshakeLen]
	if len(body) < clientHelloHeaderLen {
		return ClientHelloInfo{}, ResultMismatch
	}

	offset := 2 + 32
	sessionIDLen := int(body[offset])
	offset++
	if len(body) < offset+sessionIDLen+2 {
		return ClientHelloInfo{}, ResultMismatch
	}
	offset += sessionIDLen

	cipherSuitesLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if cipherSuitesLen == 0 || len(body) < offset+cipherSuitesLen+1 {
		return ClientHelloInfo{}, ResultMismatch
	}
	offset += cipherSuitesLen

	compressionMethodsLen := int(body[offset])
	offset++
	if compressionMethodsLen == 0 || len(body) < offset+compressionMethodsLen {
		return ClientHelloInfo{}, ResultMismatch
	}
	offset += compressionMethodsLen
	if len(body) == offset {
		return ClientHelloInfo{RecordLen: recordLen}, ResultMatch
	}
	if len(body) < offset+2 {
		return ClientHelloInfo{}, ResultMismatch
	}

	extensionsLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if len(body) < offset+extensionsLen {
		return ClientHelloInfo{}, ResultMismatch
	}

	info := ClientHelloInfo{RecordLen: recordLen}
	extensions := body[offset : offset+extensionsLen]
	for len(extensions) >= 4 {
		extType := binary.BigEndian.Uint16(extensions[:2])
		extLen := int(binary.BigEndian.Uint16(extensions[2:4]))
		extensions = extensions[4:]
		if len(extensions) < extLen {
			return ClientHelloInfo{}, ResultMismatch
		}
		extBody := extensions[:extLen]
		extensions = extensions[extLen:]
		if extType != extensionTypeServerName {
			continue
		}
		name, ok := parseServerNameExtension(extBody)
		if !ok {
			return ClientHelloInfo{}, ResultMismatch
		}
		info.ServerName = normalizeServerName(name)
		break
	}
	if len(extensions) != 0 {
		return ClientHelloInfo{}, ResultMismatch
	}
	return info, ResultMatch
}

func parseServerNameExtension(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	listLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if len(data) != listLen {
		return "", false
	}
	for len(data) >= 3 {
		nameType := data[0]
		nameLen := int(binary.BigEndian.Uint16(data[1:3]))
		data = data[3:]
		if len(data) < nameLen {
			return "", false
		}
		name := data[:nameLen]
		data = data[nameLen:]
		if nameType != serverNameTypeHostName {
			continue
		}
		if len(name) == 0 {
			return "", false
		}
		return string(name), true
	}
	return "", len(data) == 0
}

func normalizeServerName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, ".")
	value = strings.TrimSuffix(value, ".")
	return value
}
