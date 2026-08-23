package adapter

import "sync"

const defaultPooledPacketBufferSize = 2048

func newPooledPacketBuffer() any {
	return make([]byte, defaultPooledPacketBufferSize)
}

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
	desiredCapacity := len(src)
	if desiredCapacity < defaultPooledPacketBufferSize {
		desiredCapacity = defaultPooledPacketBufferSize
	}
	// Do not let a rare jumbo packet turn every later MTU-sized packet into a
	// 64 KiB allocation. Oversized pooled buffers are left for the GC.
	tooLarge := cap(buf) > desiredCapacity && cap(buf)-desiredCapacity > desiredCapacity
	if cap(buf) < len(src) || tooLarge {
		buf = make([]byte, desiredCapacity)
	}
	backing = buf[:cap(buf)]
	data = backing[:len(src)]
	copy(data, src)
	return data, backing
}
