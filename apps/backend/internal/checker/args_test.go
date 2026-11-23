package checker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgsRequiresBothArguments(t *testing.T) {
	if _, err := ParseArgs("", "out.jsonl"); !errors.Is(err, ErrMissingDataDir) {
		t.Fatalf("err = %v, want ErrMissingDataDir", err)
	}
	if _, err := ParseArgs("data", ""); !errors.Is(err, ErrMissingPredictions) {
		t.Fatalf("err = %v, want ErrMissingPredictions", err)
	}
}

func TestParseArgsRejectsMissingOrNonDirectoryData(t *testing.T) {
	dir := t.TempDir()
	if _, err := ParseArgs(filepath.Join(dir, "does-not-exist"), "out.jsonl"); !errors.Is(err, ErrDataDirNotDirectory) {
		t.Fatalf("err = %v, want ErrDataDirNotDirectory", err)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseArgs(file, "out.jsonl"); !errors.Is(err, ErrDataDirNotDirectory) {
		t.Fatalf("err = %v, want ErrDataDirNotDirectory", err)
	}
}

func TestParseArgsRejectsDirectoryPredictionsPath(t *testing.T) {
	dir := t.TempDir()
	if _, err := ParseArgs(dir, dir); !errors.Is(err, ErrPredictionsIsDir) {
		t.Fatalf("err = %v, want ErrPredictionsIsDir", err)
	}
}

func TestParseArgsAcceptsValidInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MODEL_OFF", "1")
	args, err := ParseArgs(dir, filepath.Join(dir, "out.jsonl"))
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !args.ModelOff {
		t.Fatal("MODEL_OFF=1 must be reflected in args")
	}
	if args.DataDir != dir {
		t.Fatalf("DataDir = %q, want %q", args.DataDir, dir)
	}
}

func TestModelOffIsFalseWithoutEnvironment(t *testing.T) {
	t.Setenv("MODEL_OFF", "")
	if ModelOff() {
		t.Fatal("MODEL_OFF unset must disable offline-only operation")
	}
}
