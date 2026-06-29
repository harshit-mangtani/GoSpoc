package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/harshit-mangtani/GoSpoc/internal/config"
	"github.com/harshit-mangtani/GoSpoc/internal/events"
	"github.com/harshit-mangtani/GoSpoc/internal/httpx"
	"github.com/harshit-mangtani/GoSpoc/internal/judge"
	"github.com/harshit-mangtani/GoSpoc/internal/problem"
	"github.com/harshit-mangtani/GoSpoc/internal/queue/redisstream"
	"github.com/harshit-mangtani/GoSpoc/internal/storage"
	"github.com/harshit-mangtani/GoSpoc/internal/submission"
	"github.com/harshit-mangtani/GoSpoc/internal/testcase"
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
	problemRepo := problem.NewRepository(pool)
	testcaseRepo := testcase.NewRepository(pool)

	sandbox := judge.NewDockerSandbox("docker")
	theJudge := judge.New(problemRepo, testcaseRepo, submissionRepo, sandbox, logger, judge.Config{
		PythonImage:   cfg.SandboxImagePython,
		GoImage:       cfg.SandboxImageGo,
		WorkDir:       cfg.SandboxWorkDir,
		OutputKB:      cfg.SandboxOutputKB,
		CompileWallMS: cfg.SandboxCompileWall,
		CompileMemKB:  cfg.SandboxCompileMem,
	})

	host, _ := os.Hostname()
	namePrefix := fmt.Sprintf("%s-%d", host, os.Getpid())

	publisher := events.NewPublisher(redisClient)
	w := worker.New(jobQueue, submissionRepo, theJudge, publisher, logger, cfg.WorkerConcurrency, namePrefix)

	w.Run(ctx)
	logger.Info("worker exited cleanly")
}
