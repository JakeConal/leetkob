package hardrule

import "context"

// CompilerArgs returns the C++17 flags, including system include paths,
// that the native analyzer needs in order to resolve the standard library.
func CompilerArgs(ctx context.Context) ([]string, error) {
	return nil, ErrNotImplemented
}

// RunAnalyzer executes the native analyzer and returns its stdout document.
// The process is the libclang boundary; this function does not link C++.
func RunAnalyzer(ctx context.Context, binary, sourcePath string, compilerArgs []string) ([]byte, error) {
	return nil, ErrNotImplemented
}
