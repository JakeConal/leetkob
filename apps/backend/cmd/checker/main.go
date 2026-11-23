// Command checker is the dataset-independent marking command required by
// the PA#3 assignment: `checker --data <dir> --predictions <file>`, packaged
// by the Dockerfile in apps/backend/checker. It discovers problem, statement,
// constraint and solution files generically and writes one JSONL prediction
// per solution/constraint pair. It never reads labels, scorer programs, test
// data or tags, and it never runs the scorer.
//
// MODEL_OFF=1 is selected before any provider client or credential exists.
// Ban-list constraints are decided by the analyzer executable bundled in the
// image (CHECKER_ANALYZER), and every semantic constraint is written unsure
// without network access. Without MODEL_OFF the shared core may call the
// guarded semantic port, but only after a priced budget reservation succeeds.
//
// Exit codes: 2 invalid invocation or invalid dataset input; 0 valid PA#3 run
// (the unmodified scorer decides 0/1 for the run outcome). The marker runs
// eval.mjs separately; this process neither invokes nor parses it.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker"
)

// runDeadline bounds the whole marking command, inside the marker's
// ten-minute limit for the offline image run.
const runDeadline = 10 * time.Minute

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

	// Selection order is part of the contract: MODEL_OFF is already read by
	// ParseArgs and again by OptionsFromEnv before any provider configuration
	// or credential access. Offline mode leaves the semantic port nil.
	options := checker.OptionsFromEnv()
	logger.Info("checker invocation validated",
		"data", args.DataDir,
		"predictions", args.PredictionsPath,
		"modelOff", options.ModelOff,
		"hardRuleAnalyzer", options.Analyzer != nil,
		"semanticPort", options.Semantic != nil,
		"pricedBudget", options.Budget != nil,
	)
	if options.ModelOff {
		logger.Info("MODEL_OFF=1 selected before provider initialization; no provider client, credential read, budget reservation or network call will occur")
	}

	ctx, cancel := context.WithTimeout(context.Background(), runDeadline)
	defer cancel()
	summary, err := checker.RunContext(ctx, args.DataDir, args.PredictionsPath, options)
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
		"pass", summary.Pass,
		"violation", summary.Violation,
		"unsure", summary.Unsure,
		"modelCalls", summary.ModelCalls,
		"unsureCauses", summary.UnsureCauses,
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
