package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

// Reload updates the engine configuration in-place without stopping packet
// processing. Only settings that do not change the sharding/queue topology are
// supported; otherwise a full restart is required.
func (e *Engine) Reload(cfg Config) error {
	if cfg.WorkerCount != len(e.workers) {
		return fmt.Errorf("reload requires restart: workers %d -> %d", len(e.workers), cfg.WorkerCount)
	}
	if cfg.WorkerQueueSize > 0 {
		for _, w := range e.workers {
			if cap(w.in) != cfg.WorkerQueueSize {
				return fmt.Errorf("reload requires restart: worker queue size %d -> %d", cap(w.in), cfg.WorkerQueueSize)
			}
		}
	}
	e.cfg = cfg
	for _, w := range e.workers {
		w.setConfig(cfg)
	}
	return nil
}

func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerErrCh := make(chan error, 1)
	var wg sync.WaitGroup
	for _, w := range e.workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			err := w.run(ctx)
			// On worker error, cancel the engine before flushing so recvLoop stops
			// enqueueing packets while we fail-open.
			if err != nil && !errors.Is(err, context.Canceled) {
				cancel()
			}

			// Best-effort fail-open on worker exit so we don't leave packets held in
			// WinDivert/NFQUEUE/divert paths during shutdown or error handling.
			flushTimeout := 5 * time.Second
			if cfg := w.cfg.Load(); cfg != nil && cfg.ShutdownFailOpenTimeout > 0 {
				flushTimeout = cfg.ShutdownFailOpenTimeout
			}
			flushCtx, flushCancel := context.WithTimeout(context.Background(), flushTimeout)
			flushErr := w.shutdownFailOpen(flushCtx)
			flushCancel()
			// Shutdown flushing is bounded. Do not fail the overall stop just because
			// we hit the guardrails during a normal shutdown.
			if errors.Is(err, context.Canceled) && (errors.Is(flushErr, context.DeadlineExceeded) || errors.Is(flushErr, ErrShutdownFailOpenLimitReached)) {
				flushErr = nil
			}
			if flushErr != nil {
				if err == nil || errors.Is(err, context.Canceled) {
					err = flushErr
				} else {
					err = errors.Join(err, flushErr)
				}
				cancel()
			}

			if err != nil && !errors.Is(err, context.Canceled) {
				select {
				case workerErrCh <- err:
				default:
				}
			}
		}(w)
	}

	recvErrCh := make(chan error, 1)
	go func() {
		recvErrCh <- e.recvLoop(ctx)
	}()

	var err error
	var recvErr error
	select {
	case err = <-workerErrCh:
		cancel()
		// Ensure recvLoop has stopped sending into worker queues before we close them.
		recvErr = <-recvErrCh
	case err = <-recvErrCh:
		recvErr = err
		cancel()
	}

	for _, w := range e.workers {
		w.close()
	}
	wg.Wait()

	// Fail-open any adapter-level pending packets before closing the handle.
	adapterFlushTimeout := 2 * time.Second
	if e.cfg.AdapterFlushTimeout > 0 {
		adapterFlushTimeout = e.cfg.AdapterFlushTimeout
	}
	flushCtx, flushCancel := context.WithTimeout(context.Background(), adapterFlushTimeout)
	flushErr := e.adapter.Flush(flushCtx)
	flushCancel()
	if errors.Is(err, context.Canceled) && errors.Is(flushErr, context.DeadlineExceeded) {
		flushErr = nil
	}
	if flushErr != nil {
		if err == nil || errors.Is(err, context.Canceled) {
			err = flushErr
		} else {
			err = errors.Join(err, flushErr)
		}
	}
	_ = e.adapter.Close()

	// If recvLoop didn't drive shutdown, wait for it to exit now.
	if recvErr == nil {
		recvErr = <-recvErrCh
	}

	// If recvLoop exited due to cancellation, prefer a worker error if one exists
	// (including shutdown flush errors).
	if errors.Is(err, context.Canceled) {
		select {
		case werr := <-workerErrCh:
			err = werr
		default:
		}
	}
	_ = recvErr

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

		payload := pkt.Payload()
		key := flow.KeyFromMeta(pkt.Meta)
		if len(payload) == 0 {
			// FIN/RST should go through the worker so flow state is cleaned up
			// promptly (ACK-only fast-path would otherwise keep the flow alive).
			if pkt.HasFlag(packet.TCPFlagFIN) || pkt.HasFlag(packet.TCPFlagRST) {
				idx := e.sharder.Index(key)
				if err := e.workers[idx].enqueue(ctx, pkt); err != nil {
					if errors.Is(err, context.Canceled) {
						if sendErr := e.adapter.Send(context.Background(), pkt); sendErr != nil {
							return sendErr
						}
						continue
					}
					return err
				}
				continue
			}

			// Avoid enqueueing ACK-only packets through the worker queue. Instead,
			// pass-through immediately and best-effort "touch" the flow so GC does
			// not evict active connections and accidentally re-process them later.
			idx := e.sharder.Index(key)
			e.workers[idx].touchFlow(key)
			if sendErr := e.adapter.Send(ctx, pkt); sendErr != nil {
				return sendErr
			}
			continue
		}

		idx := e.sharder.Index(key)
		if err := e.workers[idx].enqueue(ctx, pkt); err != nil {
			if errors.Is(err, context.Canceled) {
				// During shutdown, fail-open by passing through any packets we
				// already captured instead of leaving them held.
				if sendErr := e.adapter.Send(context.Background(), pkt); sendErr != nil {
					return sendErr
				}
				continue
			}
			return err
		}
	}
}
