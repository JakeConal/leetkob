package checker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// Summary reports what one checker run produced. It contains no verdict
// quality claim: the cause map explains why pairs stayed unsure so an
// operator can distinguish an honest offline run from a broken one.
type Summary struct {
	Problems    int
	Solutions   int
	Constraints int
	Predictions int
	Pass        int
	Violation   int
	Unsure      int
	// ModelCalls counts semantic dispatch attempts. MODEL_OFF must leave
	// this at zero.
	ModelCalls int
	// UnsureCauses counts unsure pairs by cause. It is diagnostic only and
	// never written to the prediction file.
	UnsureCauses map[string]int
}

// Run discovers the dataset and writes predictions using the environment
// selection: MODEL_OFF first, then the bundled analyzer, then (only when
// model use is enabled) the guarded semantic port and a priced budget.
func Run(dataDir, predictionsPath string) (Summary, error) {
	return RunContext(context.Background(), dataDir, predictionsPath, OptionsFromEnv())
}

// RunContext is the injectable entry point used by the worker and tests.
// The caller owns cancellation; the marking command applies the ten-minute
// bound.
func RunContext(ctx context.Context, dataDir, predictionsPath string, options Options) (Summary, error) {
	return runWithOptions(ctx, dataDir, predictionsPath, defaultFileOps, options)
}

// runWithOps keeps the task 7.1 test seam: a zero Options value is offline-
// safe and transport-free (no analyzer, no semantic port, no budget).
func runWithOps(dataDir, predictionsPath string, ops fileOps) (Summary, error) {
	return runWithOptions(context.Background(), dataDir, predictionsPath, ops, Options{})
}

func runWithOptions(ctx context.Context, dataDir, predictionsPath string, ops fileOps, options Options) (Summary, error) {
	problems, err := discoverDataset(dataDir, ops)
	if err != nil {
		return Summary{}, err
	}
	return writePredictions(ctx, problems, predictionsPath, ops, newEngine(options))
}

func writePredictions(ctx context.Context, problems []Problem, predictionsPath string, ops fileOps, engine *engine) (Summary, error) {
	summary := Summary{Problems: len(problems)}
	for _, problem := range problems {
		summary.Solutions += len(problem.Solutions)
		summary.Constraints += len(problem.Constraints)
	}

	outputDir := filepath.Dir(predictionsPath)
	temp, err := os.CreateTemp(outputDir, ".checker-predictions-*.tmp")
	if err != nil {
		return Summary{}, fmt.Errorf("checker: create prediction output: %w", err)
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = temp.Close()
			_ = os.Remove(tempPath)
		}
	}()

	writer := bufio.NewWriterSize(temp, 64*1024)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	for _, problem := range problems {
		statement, err := readProblemStatement(problem.StatementPath, ops)
		if err != nil {
			return Summary{}, err
		}
		for _, solution := range problem.Solutions {
			if err := ctx.Err(); err != nil {
				return Summary{}, fmt.Errorf("checker: run cancelled: %w", err)
			}
			source, err := readSolutionSource(solution.Path, ops)
			if err != nil {
				return Summary{}, err
			}
			predictions, err := engine.evaluateSolution(ctx, solution, problem.Constraints, statement, source, &summary)
			if err != nil {
				return Summary{}, err
			}
			for _, prediction := range predictions {
				if err := encoder.Encode(prediction); err != nil {
					return Summary{}, fmt.Errorf("checker: write prediction: %w", err)
				}
				summary.Predictions++
			}
		}
	}

	if err := writer.Flush(); err != nil {
		return Summary{}, fmt.Errorf("checker: flush prediction output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return Summary{}, fmt.Errorf("checker: close prediction output: %w", err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return Summary{}, fmt.Errorf("checker: set prediction permissions: %w", err)
	}
	if err := os.Rename(tempPath, predictionsPath); err != nil {
		return Summary{}, fmt.Errorf("checker: publish prediction output: %w", err)
	}
	keep = true
	return summary, nil
}

// readProblemStatement validates and reads one bounded statement. The text is
// sanitized before any semantic dispatch.
func readProblemStatement(path string, ops fileOps) (string, error) {
	info, err := ops.stat(path)
	if err != nil {
		return "", fmt.Errorf("checker: stat statement %s: %w", path, err)
	}
	if info.Size() > maxStatementBytes {
		return "", fmt.Errorf("%w: %s (%d bytes)", ErrStatementTooLarge, path, info.Size())
	}
	data, err := ops.readFile(path)
	if err != nil {
		return "", fmt.Errorf("checker: read statement %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatement, path)
	}
	return string(data), nil
}

// readSolutionSource validates and reads one bounded source file. Comments
// are analyzed by the hard rules and stripped by the sanitizer before any
// semantic dispatch; the original text is never sent unsanitized.
func readSolutionSource(path string, ops fileOps) (string, error) {
	info, err := ops.stat(path)
	if err != nil {
		return "", fmt.Errorf("checker: stat solution %s: %w", path, err)
	}
	if info.Size() > maxSourceBytes {
		return "", fmt.Errorf("%w: %s (%d bytes)", ErrSourceTooLarge, path, info.Size())
	}
	data, err := ops.readFile(path)
	if err != nil {
		return "", fmt.Errorf("checker: read solution %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: %s", ErrInvalidSource, path)
	}
	return string(data), nil
}
