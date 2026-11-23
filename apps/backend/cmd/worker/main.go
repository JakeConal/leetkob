// Command worker is the separate judging worker process required by the
// PA#3 assignment: the code queue is drained here, never inside the API
// process. It leases durable PostgreSQL jobs, compiles and runs each
// submission only through the isolated sandbox, stores one verdict set under
// the attempt fence, and records bounded infrastructure retries.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
	"github.com/JakeConal/leetkob/backend/internal/worker"
)

const (
	defaultWorkerID      = "worker-1"
	defaultPollSeconds   = 2
	defaultLeaseSeconds  = 1800
	defaultMaxFailures   = 3
	defaultWorkerBatch   = 1
	defaultShutdownSecs  = 30
	workerShutdownBudget = 2 * time.Minute
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("command", "worker")
	if err := run(logger); err != nil {
		logger.Error("worker exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("worker shut down cleanly")
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required: the worker leases durable queue jobs")
	}
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := postgres.Migrate(ctx, store.Pool()); err != nil {
		return fmt.Errorf("apply additive migrations: %w", err)
	}

	cfg := sandbox.DefaultConfig()
	cfg.Image = envOr("WORKER_SANDBOX_IMAGE", sandbox.DefaultImage)
	controller, err := sandbox.New(cfg)
	if err != nil {
		return fmt.Errorf("configure sandbox: %w", err)
	}

	workerID := envOr("WORKER_ID", defaultWorkerID)
	instance, err := worker.New(worker.Options{
		Queue:           store,
		Inputs:          store,
		Judge:           controller,
		Logger:          logger,
		WorkerID:        workerID,
		Batch:           envInt("WORKER_BATCH", defaultWorkerBatch),
		LeaseFor:        time.Duration(envInt("WORKER_LEASE_SECONDS", defaultLeaseSeconds)) * time.Second,
		MaxFailures:     envInt("WORKER_MAX_FAILURES", defaultMaxFailures),
		PollInterval:    time.Duration(envInt("WORKER_POLL_SECONDS", defaultPollSeconds)) * time.Second,
		ShutdownTimeout: time.Duration(envInt("WORKER_SHUTDOWN_SECONDS", defaultShutdownSecs)) * time.Second,
	})
	if err != nil {
		return err
	}
	logger.Info("durable queue configured", "workerId", workerID,
		"image", cfg.Image, "database", redactDatabaseURL(databaseURL))

	// Run installs its own shutdown handling; the signal context bounds the
	// lease loop, and a hard budget prevents a wedged daemon from keeping
	// the process alive forever.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- instance.Run(runCtx) }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; stopping leases")
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(workerShutdownBudget):
			return errors.New("worker did not stop within the shutdown budget")
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

// redactDatabaseURL removes any password before a connection URL is logged.
func redactDatabaseURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "configured"
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword(parsed.User.Username(), "***")
	}
	return parsed.String()
}
