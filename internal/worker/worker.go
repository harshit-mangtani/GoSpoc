package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/events"
	"github.com/harshit-mangtani/GoSpoc/internal/judge"
	"github.com/harshit-mangtani/GoSpoc/internal/queue"
	"github.com/harshit-mangtani/GoSpoc/internal/submission"
)

const opTimeout = 2 * time.Minute

type Worker struct {
	queue       queue.Queue
	store       *submission.Repository
	judger      *judge.Judge
	publisher   *events.Publisher
	logger      *slog.Logger
	concurrency int
	namePrefix  string

	wg sync.WaitGroup
}

func New(q queue.Queue, store *submission.Repository, judger *judge.Judge, publisher *events.Publisher, logger *slog.Logger, concurrency int, namePrefix string) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	if namePrefix == "" {
		namePrefix = "worker"
	}
	return &Worker{
		queue:       q,
		store:       store,
		judger:      judger,
		publisher:   publisher,
		logger:      logger,
		concurrency: concurrency,
		namePrefix:  namePrefix,
	}
}

func (w *Worker) publish(ctx context.Context, e events.Event) {
	if w.publisher == nil {
		return
	}
	if err := w.publisher.Publish(ctx, e); err != nil {
		w.logger.Error("publish event failed", "submission_id", e.SubmissionID, "error", err)
	}
}

// Run starts the consumer goroutines and blocks until ctx is cancelled and each
// has finished its current job.
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("worker started", "concurrency", w.concurrency)

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

		w.process(name, msg)
	}
}

// process drives one submission running -> done. It uses a detached context so a
// shutdown mid-job still finishes cleanly rather than leaving it stuck.
func (w *Worker) process(consumer string, msg queue.Message) {
	id := msg.Job.SubmissionID
	log := w.logger.With("submission_id", id, "consumer", consumer)

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	claimed, err := w.store.MarkRunning(ctx, id)
	if err != nil {
		log.Error("mark running failed", "error", err)
		_ = w.queue.Nack(ctx, msg)
		return
	}
	if !claimed {
		log.Info("submission not claimable (not queued), skipping")
		_ = w.queue.Ack(ctx, msg)
		return
	}
	w.publish(ctx, events.Event{SubmissionID: id, Status: "running"})

	log.Info("judging")
	rep, err := w.judger.Run(ctx, id)
	if err != nil {
		log.Error("judge failed", "error", err)
		if _, ferr := w.store.MarkFailed(ctx, id); ferr != nil {
			log.Error("mark failed failed", "error", ferr)
		}
		w.publish(ctx, events.Event{SubmissionID: id, Status: "failed"})
		_ = w.queue.Ack(ctx, msg)
		return
	}

	if _, err := w.store.MarkDone(ctx, id, rep.Verdict, rep.RuntimeMS, rep.MemoryKB); err != nil {
		log.Error("mark done failed", "error", err)
		_ = w.queue.Nack(ctx, msg)
		return
	}

	verdict, rt, mem := rep.Verdict, rep.RuntimeMS, rep.MemoryKB
	w.publish(ctx, events.Event{SubmissionID: id, Status: "done", Verdict: &verdict, RuntimeMS: &rt, MemoryKB: &mem})
	_ = w.queue.Ack(ctx, msg)
	log.Info("submission judged", "verdict", rep.Verdict, "runtime_ms", rep.RuntimeMS, "memory_kb", rep.MemoryKB)
}
