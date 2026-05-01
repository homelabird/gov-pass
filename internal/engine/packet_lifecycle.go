package engine

import (
	"context"

	"fk-gov/internal/adapter"
	"fk-gov/internal/packet"
)

func sendPacket(ctx context.Context, ad adapter.Adapter, pkt *packet.Packet) error {
	if pkt == nil {
		return nil
	}
	if err := ad.Send(ctx, pkt); err != nil {
		return err
	}
	pkt.Release()
	return nil
}
