package checker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

// writePredictions writes the JSONL artifact atomically: a temporary file
// in the destination directory is renamed only after every line is
// validated and flushed, so a failed run never leaves partial output.
func writePredictions(path string, predictions []schema.Prediction) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".checker-predictions-*.tmp")
	if err != nil {
		return fmt.Errorf("checker: create prediction output: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		os.Remove(tempName)
	}

	writer := bufio.NewWriter(temp)
	for _, prediction := range predictions {
		line, err := json.Marshal(prediction)
		if err != nil {
			cleanup()
			return fmt.Errorf("checker: encode prediction: %w", err)
		}
		if _, err := writer.Write(line); err != nil {
			cleanup()
			return fmt.Errorf("checker: write prediction: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			cleanup()
			return fmt.Errorf("checker: write newline: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		cleanup()
		return fmt.Errorf("checker: flush predictions: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("checker: close predictions: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("checker: publish predictions: %w", err)
	}
	// Predictions are non-secret evidence; the marker and the test harness
	// must be able to read them even though the image runs as a foreign uid.
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("checker: make predictions readable: %w", err)
	}
	return nil
}
