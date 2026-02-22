package reassembly

import (
	"errors"
	"sort"
)

var ErrBufferFull = errors.New("reassembly buffer full")
var ErrOutOfWindow = errors.New("fragment outside reassembly window")

type segment struct {
	offset uint32
	data   []byte
}

func (s segment) end() uint32 {
	return s.offset + uint32(len(s.data))
}

// Buffer reassembles TCP payload into a contiguous prefix from baseSeq.
// It accepts out-of-order and overlapping fragments and merges them.
type Buffer struct {
	baseSeq   uint32
	contig    []byte
	contigLen uint32
	maxBytes  uint32

	totalBytes    uint32
	segments      []segment
	hadOutOfOrder bool
	hadOverlap    bool
}

func New(baseSeq uint32, maxBytes uint32) *Buffer {
	return &Buffer{
		baseSeq:  baseSeq,
		maxBytes: maxBytes,
	}
}

// Push adds a fragment and updates the contiguous prefix if possible.
func (b *Buffer) Push(seq uint32, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if b.maxBytes == 0 {
		return ErrBufferFull
	}

	offset := seq - b.baseSeq
	if offset >= b.maxBytes {
		return ErrOutOfWindow
	}

	contigLen := b.contigLen
	end := offset + uint32(len(payload))
	if end <= contigLen {
		return nil
	}
	if offset < contigLen {
		trim := contigLen - offset
		payload = payload[trim:]
		offset = contigLen
		b.hadOverlap = true
		end = offset + uint32(len(payload))
		if len(payload) == 0 {
			return nil
		}
	}

	if end > b.maxBytes {
		return ErrBufferFull
	}
	if offset != contigLen {
		b.hadOutOfOrder = true
	}

	if offset == contigLen && len(b.segments) == 0 {
		if b.totalBytes+uint32(len(payload)) > b.maxBytes {
			return ErrBufferFull
		}
		b.contig = append(b.contig, payload...)
		b.contigLen += uint32(len(payload))
		b.totalBytes += uint32(len(payload))
		return nil
	}

	data := make([]byte, len(payload))
	copy(data, payload)
	newSeg := segment{offset: offset, data: data}

	segs := b.segments
	i := sort.Search(len(segs), func(i int) bool { return segs[i].offset >= newSeg.offset })
	left := i
	if i > 0 && segs[i-1].end() >= newSeg.offset {
		left = i - 1
	}

	newStart := newSeg.offset
	newEnd := newSeg.end()
	if left < i {
		if segs[left].offset < newStart {
			newStart = segs[left].offset
		}
		if segs[left].end() > newEnd {
			newEnd = segs[left].end()
		}
	}

	right := i
	for right < len(segs) {
		if segs[right].offset > newEnd {
			break
		}
		if segs[right].end() > newEnd {
			newEnd = segs[right].end()
		}
		right++
	}
	if right > left {
		b.hadOverlap = true
	}

	var overlapBytes uint32
	for j := left; j < right; j++ {
		overlapBytes += uint32(len(segs[j].data))
	}

	mergedLen := newEnd - newStart
	if b.totalBytes-overlapBytes+mergedLen > b.maxBytes {
		return ErrBufferFull
	}

	merged := make([]byte, mergedLen)
	for j := left; j < right; j++ {
		seg := segs[j]
		copy(merged[seg.offset-newStart:], seg.data)
	}
	copy(merged[newSeg.offset-newStart:], newSeg.data)
	newSeg = segment{offset: newStart, data: merged}

	segs = append(segs[:left], append([]segment{newSeg}, segs[right:]...)...)
	b.segments = segs
	b.totalBytes = b.totalBytes - overlapBytes + mergedLen

	b.compact()
	return nil
}

func (b *Buffer) compact() {
	for len(b.segments) > 0 && b.segments[0].offset == b.contigLen {
		seg := b.segments[0]
		b.contig = append(b.contig, seg.data...)
		b.contigLen += uint32(len(seg.data))
		b.segments = b.segments[1:]
	}
}

func (b *Buffer) Contiguous() []byte {
	return b.contig
}

func (b *Buffer) ContigLen() uint32 {
	return b.contigLen
}

func (b *Buffer) TotalBytes() uint32 {
	return b.totalBytes
}

func (b *Buffer) HadOutOfOrder() bool {
	return b.hadOutOfOrder
}

func (b *Buffer) HadOverlap() bool {
	return b.hadOverlap
}
