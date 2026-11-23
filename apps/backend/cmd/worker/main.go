// Command worker leases durable judge jobs and runs them outside the API
// process. A stop, a later submission, and a restart leave the queued job
// to be judged here. Acknowledgement of a submission does not wait for this
// process, and a stale attempt cannot commit.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/judge"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("command", "worker")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger, os.Getenv); err != nil {
		logger.Error("worker exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown signal received; worker stopped")
}

func run(ctx context.Context, logger *slog.Logger, getenv func(string) string) error {
	databaseURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("worker: DATABASE_URL is required")
	}
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Initialize(ctx); err != nil {
		return err
	}
	logger.Info("worker leasing judge jobs")
	worker := &judge.Worker{
		Queue: store,
		Exec: &judge.SandboxJudge{
			Runtime: sandbox.New(),
			Loader:  store,
		},
		ID:       workerID(),
		LeaseFor: 15 * time.Minute,
		Now:      func() time.Time { return time.Now().UTC() },
		Log:      logger,
	}
	return worker.Run(ctx)
}

func workerID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "worker"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker"
	}
	return host + "-" + hex.EncodeToString(buf[:])
}
