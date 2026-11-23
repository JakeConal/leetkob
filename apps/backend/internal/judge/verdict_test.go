package judge

import (
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// These cases lock the mapping. They construct observations directly, so a
// change in Classify-equivalent rules fails here even if the sandbox
// controller is untouched.

func TestMemoryRENoteRecordsOOMBit(t *testing.T) {
	if !strings.Contains(MemoryRENote, "OOMKilled") || !strings.Contains(MemoryRENote, "MLE") || !strings.Contains(MemoryRENote, "RE") {
		t.Fatalf("note = %q", MemoryRENote)
	}
	if !strings.Contains(MemoryRENote, "SIGKILL") || !strings.Contains(MemoryRENote, "TLE") {
		t.Fatalf("note = %q", MemoryRENote)
	}
}

func TestCompileFailuresAreCE(t *testing.T) {
	cases := []sandbox.Observation{
		{ExitCode: 1},
		{ExitCode: 0, WallTimedOut: true},
		{ExitCode: 137, OOMKilled: true},
		{ExitCode: 0, OutputLimited: true},
		{ExitCode: 0},
	}
	for _, obs := range cases {
		verdict, failed := CompileVerdict(obs, false)
		if verdict != VerdictCE || !failed {
			t.Fatalf("obs %+v → %s failed=%v, want CE", obs, verdict, failed)
		}
	}
	if verdict, failed := CompileVerdict(sandbox.Observation{ExitCode: 0}, true); failed || verdict != "" {
		t.Fatalf("successful compile → %s failed=%v", verdict, failed)
	}
}

func TestExecuteMapping(t *testing.T) {
	cases := []struct {
		name     string
		obs      sandbox.Observation
		expected string
		want     string
	}{
		{name: "oom is mle even with signal status", obs: sandbox.Observation{ExitCode: 137, OOMKilled: true, WallTimedOut: true}, want: VerdictMLE},
		{name: "wall is tle", obs: sandbox.Observation{ExitCode: 137, WallTimedOut: true}, want: VerdictTLE},
		{name: "cpu signal without oom bit is tle", obs: sandbox.Observation{ExitCode: 137}, want: VerdictTLE},
		{name: "sigxcpu is tle", obs: sandbox.Observation{ExitCode: 152}, want: VerdictTLE},
		{name: "output cap is re", obs: sandbox.Observation{ExitCode: 0, OutputLimited: true, Stdout: []byte("1 2 3")}, expected: "1 2 3", want: VerdictRE},
		{name: "normal nonzero is re", obs: sandbox.Observation{ExitCode: 1, Stdout: []byte("1")}, expected: "1", want: VerdictRE},
		{name: "segfault signal is tle under the documented signal rule", obs: sandbox.Observation{ExitCode: 139}, want: VerdictTLE},
		{name: "clean matching tokens are ac", obs: sandbox.Observation{Stdout: []byte("1\n2\t3\n")}, expected: "  1   2 3  ", want: VerdictAC},
		{name: "clean mismatched tokens are wa", obs: sandbox.Observation{Stdout: []byte("1 2")}, expected: "1 2 3", want: VerdictWA},
		{name: "numeric text is not coerced", obs: sandbox.Observation{Stdout: []byte("01")}, expected: "1", want: VerdictWA},
		{name: "empty matches empty", obs: sandbox.Observation{Stdout: []byte(" \n")}, expected: "", want: VerdictAC},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExecuteVerdict(tc.obs, tc.expected)
			if got != tc.want {
				t.Fatalf("ExecuteVerdict = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAggregatePicksMostSevere(t *testing.T) {
	got := Aggregate([]TestResult{
		{Verdict: VerdictAC},
		{Verdict: VerdictWA},
		{Verdict: VerdictTLE},
		{Verdict: VerdictRE},
	})
	if got != VerdictTLE {
		t.Fatalf("aggregate = %s, want TLE", got)
	}
	if Aggregate([]TestResult{{Verdict: VerdictWA}, {Verdict: VerdictMLE}}) != VerdictMLE {
		t.Fatal("MLE must outrank WA")
	}
	if Aggregate([]TestResult{{Verdict: VerdictAC}, {Verdict: VerdictAC}}) != VerdictAC {
		t.Fatal("all AC must stay AC")
	}
}

func TestTokenChangeIsWANotAC(t *testing.T) {
	if TokensEqual("1 2 3", "1 2 4") {
		t.Fatal("different tokens compared equal")
	}
	if !TokensEqual("a\nb", "a b") {
		t.Fatal("same tokens compared different")
	}
}
