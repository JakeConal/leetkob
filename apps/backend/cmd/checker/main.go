// Command checker is the dataset-independent marking command required by
// the PA#3 assignment: `checker --data <dir> --predictions <file>`, packaged
// by the Dockerfile in apps/backend/checker. It discovers problem, statement,
// constraint and solution files generically and writes one JSONL prediction per
// solution/constraint pair. It never reads labels, scorer programs, test data
// or tags.
//
// Task 7.1 delivers discovery and output validity. The hard-rule analyzer
// (task 5.1) and semantic model pipeline (tasks 5.3, 5.4 and 7.2) are not wired
// yet, so every prediction is honestly "unsure" with empty evidence until task
// 7.2 supplies verdicts. MODEL_OFF=1 is selected before any provider could
// exist, and this command performs no network access in any mode.
//
// Exit codes: 2 invalid invocation or invalid dataset input; 0 valid PA#3 run
// (the unmodified scorer decides 0/1 for the run outcome); never fabricate a
// run.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/JakeConal/leetkob/backend/internal/checker"
)

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

	logger.Info("checker invocation validated",
		"data", args.DataDir,
		"predictions", args.PredictionsPath,
		"modelOff", args.ModelOff,
	)
	if args.ModelOff {
		logger.Info("MODEL_OFF=1 selected before provider initialization; no network or credential access will occur")
	} else {
		logger.Info("model pipeline is not wired in task 7.1; run is network-free and verdicts are unsure")
	}

	summary, err := checker.Run(args.DataDir, args.PredictionsPath)
	if err != nil {
		logger.Error("checker run failed", "error", err)
		fmt.Fprintf(os.Stderr, "checker: %v\n", err)
		os.Exit(2)
	}

	logger.Info("checker run complete",
		"problems", summary.Problems,
		"solutions", summary.Solutions,
		"constraints", summary.Constraints,
		"predictions", summary.Predictions,
		"unsure", summary.Unsure,
		"verdictSource", "task-7.1-unsure-only; hard rules task 5.1 and model task 7.2 pending",
	)
	os.Exit(0)
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
