// Command api is the PA#3 production API process. It currently serves
// the health contract; the authenticated /v2 route families arrive with
// issue #38 tasks 2.2-2.4 and 4-6, built against the published OpenAPI
// contract. The PA#2 compatibility API keeps running separately and is
// never registered against production storage.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/httpapi"
)

const defaultPort = 8080

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

	health := &httpapi.HealthHandler{}
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		store, err := postgres.New(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Initialize(ctx); err != nil {
			return err
		}
		health.Pinger = store
		logger.Info("durable store initialized")
	} else {
		logger.Info("no DATABASE_URL configured; health reports process state only")
	}

	mux := http.NewServeMux()
	// Register without a method restriction so every method reaches the
	// handler and wrong-method requests get the contract's problem body
	// instead of the mux's plain-text 405.
	mux.Handle("/healthz", health)

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
		Handler:           mux,
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
