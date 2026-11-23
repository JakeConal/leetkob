// Command api is the PA#3 production API process. It serves the health
// contract and, when a durable store is configured, the /v2 session, class
// and lab-authoring routes; the remaining /v2 families arrive with issue #38
// tasks 4-6, built against the published OpenAPI contract. The PA#2
// compatibility API keeps running separately and is never registered
// against production storage.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

const (
	defaultPort   = 8080
	defaultOrigin = "http://127.0.0.1:3000"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("command", "api")
	if err := run(logger); err != nil {
		logger.Error("api exited with error", "error", err)
		os.Exit(1)
	}
	logger.Info("api shut down cleanly")
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps := httpapi.Deps{
		WebOrigin:     envOr("WEB_ORIGIN", defaultOrigin),
		SecureCookies: os.Getenv("SECURE_COOKIES") == "1",
	}
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		store, err := postgres.New(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := postgres.Migrate(ctx, store.Pool()); err != nil {
			return fmt.Errorf("apply additive migrations: %w", err)
		}
		deps.Pinger = store
		deps.Auth = auth.NewService(store, auth.SystemClock{}, identity.VerifyPassword)
		deps.Labs = store
		deps.Idempotency = store
		deps.Submissions = store
		deps.SubmissionReader = store
		deps.Reviews = store
		logger.Info("durable store configured; additive migrations applied")
	} else {
		logger.Info("no DATABASE_URL configured; health reports process state only")
	}

	port := defaultPort
	if raw := os.Getenv("PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return errors.New("invalid PORT: expected an integer between 1 and 65535")
		}
		port = parsed
	}

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           httpapi.NewServer(deps),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "port", port)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
