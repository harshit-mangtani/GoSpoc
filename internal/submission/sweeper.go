package submission

import (
	"context"
	"log/slog"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/queue"
)

// Sweeper periodically re-enqueues submissions that are still "queued" but
// haven't been picked up within staleAfter. It exists because Enqueue is
// best-effort: the DB row is the source of truth, and the sweeper is the
// backstop that guarantees every queued submission eventually reaches the
// queue even if the original enqueue failed or the message was lost.
type Sweeper struct {
	repo       *Repository
	queue      queue.Queue
	logger     *slog.Logger
	interval   time.Duration
	staleAfter time.Duration
}

func NewSweeper(repo *Repository, q queue.Queue, logger *slog.Logger, interval, staleAfter time.Duration) *Sweeper {
	return &Sweeper{
		repo:       repo,
		queue:      q,
		logger:     logger,
		interval:   interval,
		staleAfter: staleAfter,
	}
}

// Run blocks until ctx is cancelled, sweeping on each tick.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("sweeper started", "interval", s.interval, "stale_after", s.staleAfter)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *Sweeper) sweep(ctx context.Context) {
	ids, err := s.repo.ListStaleQueued(ctx, s.staleAfter)
	if err != nil {
		s.logger.Error("sweep: failed to list stale queued submissions", "error", err)
		return
	}

	if len(ids) == 0 {
		return
	}

	s.logger.Info("sweep: re-enqueuing stale submissions", "count", len(ids))

	for _, id := range ids {
		if err := s.queue.Enqueue(ctx, queue.Job{SubmissionID: id}); err != nil {
			s.logger.Error("sweep: re-enqueue failed", "submission_id", id, "error", err)
		}
	}
}
