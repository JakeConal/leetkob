package checker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
)

// writePredictions writes the JSONL artifact atomically: a temporary file
// in the destination directory is renamed only after every line is
// validated and flushed, so a failed run never leaves partial output.
func writePredictions(path string, predictions []schema.Prediction) error {
	directory := filepath.Dir(path)
	// A unique exclusive file with the final mode avoids a separate chmod
	// on the published artifact; rename keeps the output atomic.
	tempName := filepath.Join(directory, fmt.Sprintf(".checker-predictions-%d.tmp", time.Now().UnixNano()))
	temp, err := os.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("checker: create prediction output: %w", err)
	}
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
	return nil
}
