package tls

type Result uint8

const (
	ResultNeedMore Result = iota
	ResultMismatch
	ResultMatch
)

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
