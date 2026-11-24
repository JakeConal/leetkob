// Command checker is the dataset-independent marking command required by
// the PA#3 assignment: `checker --data <dir> --predictions <file>` with a
// Dockerfile and the MODEL_OFF=1 switch. It never reads labels, tags,
// tests or the scorer, and the offline image never calls a model: ban-list
// constraints are decided by the bundled libclang analyzer and semantic
// constraints are written as unsure.
//
// Exit codes: 2 invalid invocation or dataset; 0 a valid run (the marker
// runs the unmodified scorer separately; its exit 0 or 1 is valid, 2 is
// invalid input). No run is ever fabricated.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker"
)

const runTimeout = 10 * time.Minute

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("command", "checker")

	dataDir := flagString("data", "")
	predictionsPath := flagString("predictions", "")

	args, err := checker.ParseArgs(dataDir, predictionsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "usage: checker --data <dataset-dir> --predictions <output.jsonl>\n")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	logger.Info("checker invocation validated",
		"data", args.DataDir,
		"predictions", args.PredictionsPath,
		"modelOff", args.ModelOff,
	)
	if args.ModelOff {
		logger.Info("MODEL_OFF=1 selected before any provider configuration; no network or credential access occurs")
	}

	engine, err := checker.NewNativeAnalyzer(ctx)
	if err != nil {
		// A missing compiler configuration makes hard rules unsure; it is
		// not a fabrication and not a crash.
		logger.Warn("hard-rule analyzer unavailable; ban-list pairs will be unsure", "error", err)
	}
	var hardRules checker.HardRuleEngine
	if engine != nil {
		hardRules = engine
	} else {
		logger.Warn("CHECKER_ANALYZER is not set; ban-list pairs will be unsure")
	}

	summary, err := checker.Run(ctx, checker.RunOptions{
		Root:        args.DataDir,
		Predictions: args.PredictionsPath,
		Engine:      hardRules,
	})
	if err != nil {
		logger.Error("checker run failed", "error", err)
		os.Exit(2)
	}
	logger.Info("checker run complete",
		"problems", summary.Problems,
		"solutions", summary.Solutions,
		"constraints", summary.Constraints,
		"predictions", summary.Predictions,
		"pass", summary.Pass,
		"violation", summary.Violation,
		"unsure", summary.Unsure,
	)
}

func flagString(name, fallback string) string {
	// Minimal positional-free flag handling without the flag package's
	// help output, so the exit-code contract stays exact.
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--"+name || arg == "-"+name {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
		}
	}
	return fallback
}
