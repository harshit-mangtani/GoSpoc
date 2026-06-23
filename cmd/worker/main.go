package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/config"
	"github.com/harshit-mangtani/GoSpoc/internal/httpx"
	"github.com/harshit-mangtani/GoSpoc/internal/queue/redisstream"
	"github.com/harshit-mangtani/GoSpoc/internal/storage"
	"github.com/harshit-mangtani/GoSpoc/internal/submission"
	"github.com/harshit-mangtani/GoSpoc/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := httpx.NewLogger(cfg.Env, cfg.LogLevel).With("service", "worker", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := storage.New(ctx, cfg)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return
	}
	defer pool.Close()
	logger.Info("database connected")

	redisClient, err := storage.NewRedis(ctx, cfg)
	if err != nil {
		logger.Error("redis connection failed", "error", err)
		return
	}
	defer redisClient.Close()
	logger.Info("redis connected")

	jobQueue, err := redisstream.New(ctx, redisClient, cfg.RedisStream, cfg.RedisGroup)
	if err != nil {
		logger.Error("queue init failed", "error", err)
		return
	}
	logger.Info("job queue ready", "stream", cfg.RedisStream, "group", cfg.RedisGroup)

	submissionRepo := submission.NewRepository(pool)

	// Unique consumer prefix per process so group bookkeeping doesn't collide.
	host, _ := os.Hostname()
	namePrefix := fmt.Sprintf("%s-%d", host, os.Getpid())

	w := worker.New(
		jobQueue,
		submissionRepo,
		logger,
		cfg.WorkerConcurrency,
		time.Duration(cfg.WorkerFakeDelayMS)*time.Millisecond,
		namePrefix,
	)

	w.Run(ctx)
	logger.Info("worker exited cleanly")
}
