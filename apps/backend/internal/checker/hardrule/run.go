package hardrule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CompilerArgs returns C++17 flags, including the system include paths
// reported by the local compiler. libclang does not run the driver, so
// those paths have to be passed through or standard-library names stay
// unresolved and the analysis is unsure.
func commandPath(name string) (string, error) {
	if filepath.IsAbs(name) {
		return name, nil
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(found)
}

func CompilerArgs(ctx context.Context) ([]string, error) {
	compiler, err := commandPath("clang++")
	if err != nil {
		compiler, err = commandPath("clang")
		if err != nil {
			return nil, fmt.Errorf("hardrule: clang is not on PATH")
		}
	}
	cmd := exec.CommandContext(ctx, compiler, "-std=c++17", "-E", "-v", "-x", "c++", "-")
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil && stderr.Len() == 0 {
		return nil, fmt.Errorf("hardrule: read compiler include paths: %w", err)
	}
	args := []string{"-std=c++17"}
	collecting := false
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.Contains(line, "#include <...> search starts here:") {
			collecting = true
			continue
		}
		if strings.Contains(line, "End of search list.") {
			break
		}
		if !collecting {
			continue
		}
		path := strings.TrimSpace(line)
		if path == "" || strings.Contains(path, "(framework directory)") {
			continue
		}
		args = append(args, "-isystem", path)
	}
	if len(args) == 1 {
		return nil, fmt.Errorf("hardrule: compiler did not report include paths")
	}
	return args, nil
}

// RunAnalyzer executes the native analyzer and returns its stdout document.
// The analyzer is a separate process. A source that exceeds the byte bound
// becomes an unsure document without being parsed.
func RunAnalyzer(ctx context.Context, binary, sourcePath string, compilerArgs []string) ([]byte, error) {
	if binary == "" || sourcePath == "" {
		return nil, fmt.Errorf("hardrule: analyzer invocation is incomplete")
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("hardrule: source path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("hardrule: source path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("hardrule: source path is not a file")
	}
	if info.Size() > MaxSourceBytes {
		payload, err := json.Marshal(Result{Version: 1, Status: "unsure", Reason: "source-too-large", References: []Ref{}})
		if err != nil {
			return nil, err
		}
		return payload, nil
	}
	cmd := exec.CommandContext(ctx, binary, append([]string{abs}, compilerArgs...)...)
	var stdout, stderr capBuffer
	stdout.max = maxDocumentBytes
	stderr.max = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("hardrule: analyzer: %w", err)
	}
	if stdout.overflow {
		return nil, fmt.Errorf("hardrule: analyzer output exceeded 1 MiB")
	}
	return stdout.buf.Bytes(), nil
}

type capBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.max {
		c.overflow = true
		return len(p), nil
	}
	return c.buf.Write(p)
}
