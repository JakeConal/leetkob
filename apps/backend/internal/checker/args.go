// Package checker holds the shared checker core used by both the offline
// marking command and the online worker's constraint checks. Task 7.1 of
// issue #38 implements the generic dataset discovery; this file pins the
// command arguments and the MODEL_OFF contract up front so the interface
// cannot drift from the rubric.
package checker

import (
	"errors"
	"fmt"
	"os"
)

// Errors returned by ParseArgs. The command maps them to exit code 2
// (invalid invocation), distinct from a valid run's exit 0/1.
var (
	ErrMissingDataDir      = errors.New("checker: --data is required")
	ErrMissingPredictions  = errors.New("checker: --predictions is required")
	ErrDataDirNotDirectory = errors.New("checker: --data must be an existing directory")
	ErrPredictionsIsDir    = errors.New("checker: --predictions must be a file path, not a directory")
	ErrDataDirNotReadable  = errors.New("checker: --data is not readable")
)

// Args are the validated command arguments. The dataset directory is
// discovered generically; no per-problem configuration exists.
type Args struct {
	DataDir         string
	PredictionsPath string
	ModelOff        bool
}

// ModelOff is true when the environment selects offline operation:
// MODEL_OFF=1 is read before any provider client or credential is
// created, disables all model/network calls, decides ban-list pairs with
// hard rules and writes unsure for every semantic pair.
func ModelOff() bool {
	return os.Getenv("MODEL_OFF") == "1"
}

// ParseArgs validates the command invocation without touching the dataset
// contents. Validation failure exits with a distinct error so the command
// can document exit code 2 separately from run outcomes.
func ParseArgs(dataDir, predictionsPath string) (Args, error) {
	if dataDir == "" {
		return Args{}, ErrMissingDataDir
	}
	if predictionsPath == "" {
		return Args{}, ErrMissingPredictions
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return Args{}, fmt.Errorf("%w: %s", ErrDataDirNotDirectory, dataDir)
	}
	if !info.IsDir() {
		return Args{}, fmt.Errorf("%w: %s", ErrDataDirNotDirectory, dataDir)
	}
	if info, err := os.Stat(predictionsPath); err == nil && info.IsDir() {
		return Args{}, fmt.Errorf("%w: %s", ErrPredictionsIsDir, predictionsPath)
	}
	return Args{DataDir: dataDir, PredictionsPath: predictionsPath, ModelOff: ModelOff()}, nil
}
