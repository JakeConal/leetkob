package checking

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
)

// AnalyzeSource writes source to a temporary file and runs the native
// analyzer. An empty binary fails before a process is started. Tests inject
// AnalyzeFunc instead of calling this helper.
func AnalyzeSource(ctx context.Context, binary string, compilerArgs []string, source string) ([]byte, error) {
	if strings.TrimSpace(source) == "" {
		return nil, errors.New("checking: source is empty")
	}
	dir, err := os.MkdirTemp("", "leetkob-check-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	written := filepath.Join(dir, "main.cpp")
	if err := os.WriteFile(written, []byte(source), 0o600); err != nil {
		return nil, err
	}
	path := written
	if real, err := filepath.EvalSymlinks(written); err == nil {
		path = real
	}
	return hardrule.RunAnalyzer(ctx, binary, path, compilerArgs)
}
