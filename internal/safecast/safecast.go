package safecast

const (
	maxUint16Value = uint32(^uint16(0))
	maxUint16Int64 = int64(^uint16(0))
	maxUint32Value = int64(^uint32(0))
)

var maxIntValue = uint64(^uint(0) >> 1)

func IntToUint16(v int) (uint16, bool) {
	if v < 0 || int64(v) > maxUint16Int64 {
		return 0, false
	}
	// #nosec G115 -- v is range-checked against maxUint16Value above.
	return uint16(v), true
}

func IntToUint32(v int) (uint32, bool) {
	if v < 0 || int64(v) > maxUint32Value {
		return 0, false
	}
	// #nosec G115 -- v is range-checked against maxUint32Value above.
	return uint32(v), true
}

func IntToUint64(v int) (uint64, bool) {
	if v < 0 {
		return 0, false
	}
	// #nosec G115 -- v is checked to be non-negative above.
	return uint64(v), true
}

func Uint32ToUint16(v uint32) (uint16, bool) {
	if v > maxUint16Value {
		return 0, false
	}
	// #nosec G115 -- v is range-checked against maxUint16Value above.
	return uint16(v), true
}

func Uint64ToInt(v uint64) (int, bool) {
	if v > maxIntValue {
		return 0, false
	}
	// #nosec G115 -- v is range-checked against maxIntValue above.
	return int(v), true
}
