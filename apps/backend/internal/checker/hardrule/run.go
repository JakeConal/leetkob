package hardrule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrToolMissing means clang or libclang is not installed. Callers skip
// the native analyzer instead of treating that as a product failure.
var ErrToolMissing = errors.New("hardrule: analyzer tool missing")

// CompileAnalyzer builds analyzer.c into destDir and returns the binary path.
func CompileAnalyzer(destDir, sourcePath string) (string, error) {
	libdir, err := libclangDir()
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrToolMissing, err.Error())
	}
	clang, err := commandPath("clang")
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrToolMissing, err.Error())
	}
	out := filepath.Join(destDir, "cpp-analyzer")
	cmd := exec.Command(clang, "-std=c11", sourcePath, "-o", out, "-lclang", "-L"+libdir, "-Wl,-rpath,"+libdir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("hardrule: compile analyzer: %w\n%s", err, output)
	}
	return out, nil
}

func libclangDir() (string, error) {
	clang, err := commandPath("clang")
	if err != nil {
		return "", err
	}
	if out, err := exec.Command(clang, "-print-file-name=libclang.dylib").Output(); err == nil {
		path := strings.TrimSpace(string(out))
		if path != "" && filepath.IsAbs(path) {
			return filepath.Dir(path), nil
		}
	}
	candidates := []string{
		"/Library/Developer/CommandLineTools/usr/lib/libclang.dylib",
		"/usr/lib/llvm-19/lib/libclang.so",
		"/usr/lib/llvm-19/lib/libclang.so.1",
		"/usr/lib/x86_64-linux-gnu/libclang-19.so",
		"/usr/lib/llvm-18/lib/libclang.so",
		"/usr/lib/libclang.so",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Dir(candidate), nil
		}
	}
	return "", os.ErrNotExist
}

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
