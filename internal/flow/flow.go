package flow

import (
	"time"

	"fk-gov/internal/packet"
	"fk-gov/internal/reassembly"
)

type Key struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
	Proto   uint8
}

func KeyFromMeta(m packet.Meta) Key {
	return Key{
		SrcIP:   m.SrcIP,
		DstIP:   m.DstIP,
		SrcPort: m.SrcPort,
		DstPort: m.DstPort,
		Proto:   m.Proto,
	}
}

type State uint8

const (
	StateNew State = iota
	StateCollecting
	StateSplitReady
	StateInjected
	StatePassThrough
	StateClosed
)

type FlowState struct {
	State           State
	BaseSeq         uint32
	LastActive      time.Time
	CollectStart    time.Time
	FirstPayloadLen int
	Template        *packet.Packet
	HeldPackets     []*packet.Packet
	Reassembler     *reassembly.Buffer
	Processed       bool
}

type Table struct {
	items map[Key]*FlowState
}

func NewTable() *Table {
	return &Table{items: make(map[Key]*FlowState)}
}

func (t *Table) Len() int {
	return len(t.items)
}

func (t *Table) Get(key Key) (*FlowState, bool) {
	st, ok := t.items[key]
	return st, ok
}

func (t *Table) GetOrCreate(key Key, now time.Time) *FlowState {
	if st, ok := t.items[key]; ok {
		return st
	}
	st := &FlowState{
		State:      StateNew,
		LastActive: now,
	}
	t.items[key] = st
	return st
}

func (t *Table) Delete(key Key) {
	delete(t.items, key)
}

func (t *Table) Range(fn func(Key, *FlowState)) {
	for k, v := range t.items {
		fn(k, v)
	}
}
