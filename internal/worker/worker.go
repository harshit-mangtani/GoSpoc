package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/queue"
)

// Store is the persistence the worker needs. *submission.Repository satisfies it.
type Store interface {
	MarkRunning(ctx context.Context, id int64) (bool, error)
	MarkDone(ctx context.Context, id int64, verdict string, runtimeMS int) (bool, error)
}

const opTimeout = 30 * time.Second

type Worker struct {
	queue       queue.Queue
	store       Store
	logger      *slog.Logger
	concurrency int
	fakeDelay   time.Duration
	namePrefix  string

	wg sync.WaitGroup
}

func New(q queue.Queue, store Store, logger *slog.Logger, concurrency int, fakeDelay time.Duration, namePrefix string) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	if namePrefix == "" {
		namePrefix = "worker"
	}
	return &Worker{
		queue:       q,
		store:       store,
		logger:      logger,
		concurrency: concurrency,
		fakeDelay:   fakeDelay,
		namePrefix:  namePrefix,
	}
}

// Run starts the consumer goroutines and blocks until ctx is cancelled and each
// has finished its current job.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker started", "concurrency", w.concurrency, "fake_delay", w.fakeDelay)

	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.runConsumer(ctx, fmt.Sprintf("%s-%d", w.namePrefix, i))
	}

	w.wg.Wait()
	w.logger.Info("worker stopped")
}

func (w *Worker) runConsumer(ctx context.Context, name string) {
	defer w.wg.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		msg, err := w.queue.Consume(ctx, name)
		if err != nil {
			if errors.Is(err, queue.ErrNoMessage) {
				continue
			}
			if ctx.Err() != nil {
				return
			}

			w.logger.Error("consume failed", "consumer", name, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		w.process(ctx, name, msg)
	}
}

// process drives one submission running -> done. Side effects use a detached
// context so a shutdown mid-job still finishes cleanly.
func (w *Worker) process(ctx context.Context, consumer string, msg queue.Message) {
	id := msg.Job.SubmissionID
	log := w.logger.With("submission_id", id, "consumer", consumer)

	opCtx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	claimed, err := w.store.MarkRunning(opCtx, id)
	if err != nil {
		log.Error("mark running failed", "error", err)
		_ = w.queue.Nack(opCtx, msg)
		return
	}
	if !claimed {
		log.Info("submission not claimable (not queued), skipping")
		_ = w.queue.Ack(opCtx, msg)
		return
	}

	log.Info("judging (fake)")
	start := time.Now()
	select {
	case <-ctx.Done():
	case <-time.After(w.fakeDelay):
	}
	runtimeMS := int(time.Since(start).Milliseconds())

	const verdict = "AC" // Phase 6 placeholder
	done, err := w.store.MarkDone(opCtx, id, verdict, runtimeMS)
	if err != nil {
		log.Error("mark done failed", "error", err)
		_ = w.queue.Nack(opCtx, msg)
		return
	}
	if !done {
		log.Warn("submission was not in 'running' state at completion")
	}

	_ = w.queue.Ack(opCtx, msg)
	log.Info("submission judged", "verdict", verdict, "runtime_ms", runtimeMS)
}
