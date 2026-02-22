package flow

import "hash/fnv"

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
	_, _ = h.Write(key.SrcIP[:])
	_, _ = h.Write(key.DstIP[:])
	var buf [5]byte
	buf[0] = byte(key.SrcPort >> 8)
	buf[1] = byte(key.SrcPort)
	buf[2] = byte(key.DstPort >> 8)
	buf[3] = byte(key.DstPort)
	buf[4] = key.Proto
	_, _ = h.Write(buf[:])
	return int(h.Sum64() % uint64(s.workers))
}
