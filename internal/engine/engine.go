package engine

import (
	"context"
	"errors"
	"sync"

	"fk-gov/internal/adapter"
	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
)

type Engine struct {
	cfg     Config
	adapter adapter.Adapter
	sharder *flow.Sharder
	workers []*worker
}

func New(cfg Config, ad adapter.Adapter) *Engine {
	sharder := flow.NewSharder(cfg.WorkerCount)
	workers := make([]*worker, sharder.Workers())
	for i := range workers {
		workers[i] = newWorker(i, cfg, ad)
	}
	return &Engine{
		cfg:     cfg,
		adapter: ad,
		sharder: sharder,
		workers: workers,
	}
}

func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, w := range e.workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			_ = w.run(ctx)
		}(w)
	}

	err := e.recvLoop(ctx)
	cancel()
	for _, w := range e.workers {
		w.close()
	}
	wg.Wait()
	_ = e.adapter.Close()

	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (e *Engine) recvLoop(ctx context.Context) error {
	for {
		pkt, err := e.adapter.Recv(ctx)
		if err != nil {
			return err
		}
		if pkt == nil {
			continue
		}

		if err := packet.DecodeIPv4TCP(pkt); err != nil {
			if sendErr := e.adapter.Send(ctx, pkt); sendErr != nil {
				return sendErr
			}
			continue
		}

		if pkt.Meta.DstPort != 443 {
			if sendErr := e.adapter.Send(ctx, pkt); sendErr != nil {
				return sendErr
			}
			continue
		}

		key := flow.KeyFromMeta(pkt.Meta)
		idx := e.sharder.Index(key)
		if err := e.workers[idx].enqueue(ctx, pkt); err != nil {
			return err
		}
	}
}
