package sandbox

import (
	"errors"
	"testing"
	"time"
)

func TestJudgeRunPolicyTriplesCPU(t *testing.T) {
	policy, err := JudgeRunPolicy(2, 256)
	if err != nil {
		t.Fatal(err)
	}
	if policy.CPUSeconds != 2 || policy.MemoryBytes != 256<<20 {
		t.Fatalf("policy = %+v", policy)
	}
	if policy.Wall != 6*time.Second {
		t.Fatalf("wall = %s, want 6s", policy.Wall)
	}
	if policy.OutputLimit != DefaultOutputBytes {
		t.Fatalf("output = %d", policy.OutputLimit)
	}
}

func TestJudgeRunPolicyRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		cpu    int
		memory int
	}{
		{0, 256},
		{6, 256},
		{1, 63},
		{1, 513},
	}
	for _, tc := range cases {
		if _, err := JudgeRunPolicy(tc.cpu, tc.memory); !errors.Is(err, ErrPolicy) {
			t.Fatalf("cpu %d memory %d: err = %v", tc.cpu, tc.memory, err)
		}
	}
}

func TestRunPolicyRejectsDisabledLimits(t *testing.T) {
	valid, err := JudgeRunPolicy(1, 256)
	if err != nil {
		t.Fatal(err)
	}
	cases := []RunPolicy{
		{CPUSeconds: valid.CPUSeconds, MemoryBytes: valid.MemoryBytes, Wall: 0, OutputLimit: valid.OutputLimit},
		{CPUSeconds: valid.CPUSeconds, MemoryBytes: valid.MemoryBytes, Wall: valid.Wall, OutputLimit: 0},
		{CPUSeconds: valid.CPUSeconds, MemoryBytes: valid.MemoryBytes, Wall: valid.Wall, OutputLimit: DefaultOutputBytes + 1},
		{CPUSeconds: valid.CPUSeconds, MemoryBytes: valid.MemoryBytes, Wall: maxWall + time.Second, OutputLimit: valid.OutputLimit},
	}
	for _, policy := range cases {
		if err := policy.validate(); !errors.Is(err, ErrPolicy) {
			t.Fatalf("policy %+v: err = %v", policy, err)
		}
	}
}

func TestCompileProfileIsLocked(t *testing.T) {
	profile := compileProfile()
	args, err := createArgs("leetkob-sbx-0123456789abcdef", ImageRef, profile, compileCommand)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertProductionArgs(args); err != nil {
		t.Fatal(err)
	}
	if flagValue(args, "--ulimit") != "cpu=10:10" {
		t.Fatalf("cpu ulimit = %s", flagValue(args, "--ulimit"))
	}
	if flagValue(args, "--memory") != "536870912" || flagValue(args, "--memory-swap") != "536870912" {
		t.Fatalf("memory flags = %s %s", flagValue(args, "--memory"), flagValue(args, "--memory-swap"))
	}
}
