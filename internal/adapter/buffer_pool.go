package adapter

import "sync"

func copyIntoPoolBuffer(pool *sync.Pool, src []byte) (data []byte, backing []byte) {
	if len(src) == 0 {
		return nil, nil
	}
	if pool == nil {
		out := append([]byte(nil), src...)
		return out, nil
	}

	var buf []byte
	switch v := pool.Get().(type) {
	case []byte:
		buf = v
	case *[]byte:
		if v != nil {
			buf = *v
		}
	}
	if cap(buf) < len(src) {
		buf = make([]byte, len(src))
	}
	backing = buf[:cap(buf)]
	data = backing[:len(src)]
	copy(data, src)
	return data, backing
}
