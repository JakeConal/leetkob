package judge

import (
	"strings"
	"unicode"

	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// Student verdicts from the published PA#3 contract.
const (
	VerdictAC  = "AC"
	VerdictWA  = "WA"
	VerdictTLE = "TLE"
	VerdictCE  = "CE"
	VerdictRE  = "RE"
	VerdictMLE = "MLE"
)

// severity picks the aggregate. CE dominates, then memory, time, runtime,
// and wrong answer. AC is the absence of every other verdict.
var severity = map[string]int{
	VerdictAC:  0,
	VerdictWA:  1,
	VerdictRE:  2,
	VerdictTLE: 3,
	VerdictMLE: 4,
	VerdictCE:  5,
}

// CompileVerdict maps a finished compilation observation. Any compile
// failure, including the compilation wall deadline and a compilation OOM,
// is CE. ok is false when the caller must not execute the binary.
func CompileVerdict(obs sandbox.Observation, binaryOK bool) (verdict string, failed bool) {
	if obs.WallTimedOut || obs.OutputLimited || obs.OOMKilled || obs.ExitCode != 0 || !binaryOK {
		return VerdictCE, true
	}
	return "", false
}

// ExecuteVerdict maps one finished execution. Token comparison runs only
// for a zero exit that was not killed for time, memory, or output.
func ExecuteVerdict(obs sandbox.Observation, expected string) string {
	switch {
	case obs.OOMKilled:
		return VerdictMLE
	case obs.WallTimedOut:
		return VerdictTLE
	case obs.OutputLimited:
		return VerdictRE
	case signalExit(obs.ExitCode):
		return VerdictTLE
	case obs.ExitCode != 0:
		return VerdictRE
	case TokensEqual(string(obs.Stdout), expected):
		return VerdictAC
	default:
		return VerdictWA
	}
}

// signalExit is a non-zero signal status. Unix records signal n as
// 128+n, so a normal exit(1) stays RE and a CPU-limit SIGKILL (137) or
// SIGXCPU is TLE when the sandbox did not set OOM, wall, or output.
func signalExit(code int) bool {
	return code > 128
}

// TokensEqual reports whether actual and expected have the same
// whitespace-separated tokens. Leading, trailing, and repeated whitespace
// are ignored. Numeric text is not rewritten.
func TokensEqual(actual, expected string) bool {
	a := strings.FieldsFunc(actual, unicode.IsSpace)
	b := strings.FieldsFunc(expected, unicode.IsSpace)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Aggregate returns the most severe per-test verdict. An empty set is AC
// only when the caller has already rejected a lab with no tests.
func Aggregate(tests []TestResult) string {
	best := VerdictAC
	for _, test := range tests {
		if severity[test.Verdict] > severity[best] {
			best = test.Verdict
		}
	}
	return best
}

// KnownVerdict reports whether verdict is one of the six student codes.
func KnownVerdict(verdict string) bool {
	_, ok := severity[verdict]
	return ok
}
