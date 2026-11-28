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
	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/checking"
	"github.com/JakeConal/leetkob/backend/internal/judge"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
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
	model, err := configureModel(getenv, readModelToken, logger)
	if err != nil {
		return err
	}
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Initialize(ctx); err != nil {
		return err
	}
	logger.Info("worker leasing judge and check jobs")
	id := workerID()
	now := func() time.Time { return time.Now().UTC() }
	judgeWorker := &judge.Worker{
		Queue: store,
		Exec: &judge.SandboxJudge{
			Runtime: sandbox.New(),
			Loader:  store,
		},
		ID:       id,
		LeaseFor: 15 * time.Minute,
		Now:      now,
		Log:      logger,
	}
	checks := &checking.Runner{
		Queue:   store,
		Service: checkService(ctx, store, getenv, logger, model),
		ID:      id,
		Now:     now,
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := judgeWorker.Process(ctx); err != nil && ctx.Err() == nil {
			logger.Error("judge batch failed", "error", err.Error())
		}
		if _, err := checks.Process(ctx); err != nil && ctx.Err() == nil {
			logger.Error("check batch failed", "error", err.Error())
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func checkService(ctx context.Context, store *postgres.Store, getenv func(string) string, logger *slog.Logger, model modelSettings) *checking.Service {
	analyze := func(context.Context, string) ([]byte, error) {
		return []byte(`{"version":1,"status":"unsure","reason":"analyzer-not-configured","references":[]}`), nil
	}
	if getenv("WORKER_ISOLATED_ANALYZER") == "1" {
		boundary := sandbox.New()
		boundary.Image = "leetkob-local-analyzer:pa3"
		analyze = boundary.Analyze
	} else if binary := strings.TrimSpace(getenv("CHECKER_ANALYZER")); binary != "" {
		args, err := hardrule.CompilerArgs(ctx)
		if err != nil {
			logger.Error("checker compiler arguments failed", "error", err.Error())
		} else {
			analyze = func(ctx context.Context, source string) ([]byte, error) {
				return checking.AnalyzeSource(ctx, binary, args, source)
			}
		}
	}
	return &checking.Service{
		Store:   &postgres.FindingRepo{Store: store},
		Analyze: analyze,
		Checker: &semantic.Checker{
			Model: model.model,
			Store: &postgres.Budget{
				Store: store, MaxLabCalls: model.maxLabCalls, MaxDailyCalls: model.maxDailyCalls,
			},
			Timeout: model.timeout,
		},
		Price: model.price,
	}
}

// offlineModel keeps the pipeline complete without a paid provider. Invalid
// output becomes unsure after the checker's one repair attempt.
type offlineModel struct{}

func (offlineModel) Complete(context.Context, semantic.CompletionRequest) (semantic.ModelResponse, error) {
	return semantic.ModelResponse{Content: `{}`, TokensKnown: true, InputTokens: 1, OutputTokens: 1, RequestID: "model-off"}, nil
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
