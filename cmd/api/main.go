package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/config"
	"github.com/harshit-mangtani/GoSpoc/internal/httpx"
)

func main() {
	cfg := config.Load()
	logger := httpx.NewLogger(cfg.Env, cfg.LogLevel).With("env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "PONG")
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter,r *http.Request){
		panic("test-panic")
	})
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: httpx.Recovery(logger)(httpx.RequestID(logger)(mux)),
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		logger.Error("server failed", "err", err)
		return
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		return
	}

	logger.Info("server stopped cleanly")
}
