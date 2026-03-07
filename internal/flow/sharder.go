package flow

import (
	"encoding/binary"
	"hash/fnv"

	"fk-gov/internal/safecast"
)

type Sharder struct {
	workers int
}

func NewSharder(workers int) *Sharder {
	if workers < 1 {
		workers = 1
	}
	return &Sharder{workers: workers}
}

func (s *Sharder) Workers() int {
	return s.workers
}

func (s *Sharder) Index(key Key) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte{key.IPVersion})
	_, _ = h.Write(key.SrcIP[:])
	_, _ = h.Write(key.DstIP[:])
	var buf [5]byte
	binary.BigEndian.PutUint16(buf[0:2], key.SrcPort)
	binary.BigEndian.PutUint16(buf[2:4], key.DstPort)
	buf[4] = key.Proto
	_, _ = h.Write(buf[:])
	workers, ok := safecast.IntToUint64(s.workers)
	if !ok || workers == 0 {
		return 0
	}
	idx, ok := safecast.Uint64ToInt(h.Sum64() % workers)
	if !ok {
		return 0
	}
	return idx
}
