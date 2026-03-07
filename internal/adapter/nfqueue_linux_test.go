//go:build linux

package adapter

import (
	"testing"

	nfqueue "github.com/florianl/go-nfqueue"

	"fk-gov/internal/packet"
)

func TestNFQueueIfIndexPrefersOutDev(t *testing.T) {
	outDev := uint32(7)
	physOutDev := uint32(9)
	got := nfqueueIfIndex(nfqueue.Attribute{
		OutDev:     &outDev,
		PhysOutDev: &physOutDev,
	})
	if got != outDev {
		t.Fatalf("expected outdev=%d, got %d", outDev, got)
	}
}

func TestNFQueueIfIndexFallsBackToPhysOutDev(t *testing.T) {
	physOutDev := uint32(9)
	got := nfqueueIfIndex(nfqueue.Attribute{
		PhysOutDev: &physOutDev,
	})
	if got != physOutDev {
		t.Fatalf("expected physOutDev=%d, got %d", physOutDev, got)
	}
}

func TestNFQueuePacketVersion(t *testing.T) {
	ad := &NFQueueAdapter{}

	if got := ad.packetVersion(&packet.Packet{Meta: packet.Meta{IPVersion: packet.IPVersion6}}); got != packet.IPVersion6 {
		t.Fatalf("expected version from meta, got %d", got)
	}
	if got := ad.packetVersion(&packet.Packet{Data: []byte{0x45}}); got != packet.IPVersion4 {
		t.Fatalf("expected version 4 from data, got %d", got)
	}
	if got := ad.packetVersion(&packet.Packet{Data: []byte{0x60}}); got != packet.IPVersion6 {
		t.Fatalf("expected version 6 from data, got %d", got)
	}
}
