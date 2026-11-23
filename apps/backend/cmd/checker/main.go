// Command checker is the dataset-independent marking command required by
// the PA#3 assignment: `checker --data <dir> --predictions <file>` with a
// Dockerfile (built with the sandbox image in issue #38 task 7.1). This
// scaffold pins the command line, the exit-code contract and the
// MODEL_OFF=1 selection point; generic dataset discovery lands with task
// 7.1 and never reads labels or scorer data.
//
// Exit codes: 2 invalid invocation; 0/1 valid PA#3 run outcomes (the
// unmodified scorer decides between them); never fabricate a run.
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
	}

	// The shared checker core (generic statement/constraint/source
	// discovery and prediction output) arrives with issue #38 task 7.1.
	logger.Warn("checker core not implemented yet (issue #38 task 7.1); arguments validated only, no predictions written")
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
