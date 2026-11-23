package checker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// Summary reports what one checker run produced. It contains no verdict
// quality claim: in task 7.1 every prediction is unsure by design.
type Summary struct {
	Problems    int
	Solutions   int
	Constraints int
	Predictions int
	Unsure      int
}

// Run discovers the dataset and writes predictions to predictionsPath. The
// output file is replaced atomically, so a failed run never leaves a partial
// file that a scorer could mistake for valid JSONL.
func Run(dataDir, predictionsPath string) (Summary, error) {
	return runWithOps(dataDir, predictionsPath, defaultFileOps)
}

func runWithOps(dataDir, predictionsPath string, ops fileOps) (Summary, error) {
	problems, err := discoverDataset(dataDir, ops)
	if err != nil {
		return Summary{}, err
	}
	return writePredictions(problems, predictionsPath, ops)
}

func writePredictions(problems []Problem, predictionsPath string, ops fileOps) (Summary, error) {
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
	// Evidence is allocated once and reused; every prediction marshals it as []
	// rather than null, which the prediction schema requires for unsure lines.
	emptyEvidence := []Evidence{}

	for _, problem := range problems {
		if err := readProblemStatement(problem.StatementPath, ops); err != nil {
			return Summary{}, err
		}
		for _, solution := range problem.Solutions {
			if err := readSolutionSource(solution.Path, ops); err != nil {
				return Summary{}, err
			}
			for _, constraint := range problem.Constraints {
				// Task 7.1 does not decide verdicts. The hard-rule analyzer
				// (task 5.1) and the semantic model pipeline (tasks 5.3, 5.4
				// and 7.2) are not wired here yet, so every pair is honestly
				// unsure with empty evidence. Task 7.2 replaces this branch
				// with hard-rule and model verdicts; it must never infer a
				// verdict from labels, scorer data or test output.
				prediction := Prediction{
					Solution:   solution.ID,
					Constraint: constraint.ID,
					Verdict:    VerdictUnsure,
					Evidence:   emptyEvidence,
				}
				if err := encoder.Encode(prediction); err != nil {
					return Summary{}, fmt.Errorf("checker: write prediction: %w", err)
				}
				summary.Predictions++
				summary.Unsure++
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

// readProblemStatement validates and reads one bounded statement. The bytes are
// unused until task 7.2 supplies the semantic model; reading here keeps
// discovery honest about unreadable or oversized statements.
func readProblemStatement(path string, ops fileOps) error {
	info, err := ops.stat(path)
	if err != nil {
		return fmt.Errorf("checker: stat statement %s: %w", path, err)
	}
	if info.Size() > maxStatementBytes {
		return fmt.Errorf("%w: %s (%d bytes)", ErrStatementTooLarge, path, info.Size())
	}
	data, err := ops.readFile(path)
	if err != nil {
		return fmt.Errorf("checker: read statement %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w: %s", ErrInvalidStatement, path)
	}
	return nil
}

// readSolutionSource validates and reads one bounded source file. The bytes are
// intentionally unused until task 7.2 supplies the analyzer; reading them here
// keeps discovery honest about unreadable or oversized inputs instead of
// silently emitting predictions for sources that cannot be analyzed.
func readSolutionSource(path string, ops fileOps) error {
	info, err := ops.stat(path)
	if err != nil {
		return fmt.Errorf("checker: stat solution %s: %w", path, err)
	}
	if info.Size() > maxSourceBytes {
		return fmt.Errorf("%w: %s (%d bytes)", ErrSourceTooLarge, path, info.Size())
	}
	data, err := ops.readFile(path)
	if err != nil {
		return fmt.Errorf("checker: read solution %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w: %s", ErrInvalidSource, path)
	}
	return nil
}
