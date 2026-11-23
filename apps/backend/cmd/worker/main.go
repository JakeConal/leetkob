// Command worker is the separate judging worker process required by the
// PA#3 assignment: the code queue is drained here, never inside the API
// process. The durable PostgreSQL queue adapter arrives with issue #38
// tasks 2.1 and 4.2; until it is configured this process starts, logs its
// state and shuts down cleanly on SIGINT/SIGTERM.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("command", "worker")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The queue is an injected domain port (internal/domain.Queue). Task 2.1
	// provides the shared store and transactions; job leasing is task 4.2.
	logger.Info("worker started; job leasing is not configured yet (issue #38 task 4.2); idling until shutdown")
	<-ctx.Done()
	logger.Info("shutdown signal received; worker stopped with no leased jobs")
}
