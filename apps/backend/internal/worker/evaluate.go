package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

// maxCompareTokens mirrors the sandbox's stdout token cap so a flood cannot
// inflate comparison memory.
const maxCompareTokens = 65536

// evaluate turns one sandbox result into the single verdict set the store
// commits. Token comparison ignores whitespace. The aggregate follows the
// documented priority: CE, then TLE, MLE, RE, WA and finally AC.
func evaluate(result sandbox.JudgeResult, tests []postgres.JudgeTest) (postgres.JudgeOutcome, error) {
	outcome := postgres.JudgeOutcome{
		Verdict: string(result.Verdict),
		Tests:   make([]postgres.TestOutcome, 0, len(tests)),
	}
	if result.Verdict == sandbox.VerdictCompileError {
		// Compilation failed, so no test executed and no per-test row is
		// stored. The aggregate CE is the complete result.
		outcome.Verdict = string(sandbox.VerdictCompileError)
		return outcome, nil
	}
	if len(result.Tests) != len(tests) {
		return postgres.JudgeOutcome{}, fmt.Errorf(
			"judge returned %d test results for %d lab tests", len(result.Tests), len(tests))
	}

	perTest := make([]string, 0, len(tests))
	for index, testResult := range result.Tests {
		verdict := perTestVerdict(testResult, tests[index].ExpectedOutput)
		perTest = append(perTest, verdict)
		outcome.Tests = append(outcome.Tests, postgres.TestOutcome{
			TestID:  tests[index].ID,
			Verdict: verdict,
			TimeMs:  elapsedMillis(testResult),
		})
	}
	aggregate, err := aggregateVerdict(perTest)
	if err != nil {
		return postgres.JudgeOutcome{}, err
	}
	outcome.Verdict = aggregate
	return outcome, nil
}

func perTestVerdict(result sandbox.TestResult, expectedOutput string) string {
	switch result.Verdict {
	case sandbox.VerdictRan:
		if sameTokens(result.Tokens, tokensOf(expectedOutput)) {
			return "AC"
		}
		return "WA"
	case sandbox.VerdictCompileError:
		return "CE"
	case sandbox.VerdictTimeLimitExceeded:
		return "TLE"
	case sandbox.VerdictMemoryLimitExceeded:
		return "MLE"
	case sandbox.VerdictRuntimeError:
		return "RE"
	default:
		// An unknown sandbox state must not be reported as Accepted.
		return "RE"
	}
}

func aggregateVerdict(perTest []string) (string, error) {
	if len(perTest) == 0 {
		return "", errors.New("judge produced no per-test verdicts")
	}
	seen := make(map[string]bool, len(perTest))
	for _, verdict := range perTest {
		seen[verdict] = true
	}
	for _, candidate := range []string{"CE", "TLE", "MLE", "RE", "WA"} {
		if seen[candidate] {
			return candidate, nil
		}
	}
	if seen["AC"] {
		return "AC", nil
	}
	return "", errors.New("judge produced an unknown verdict")
}

func elapsedMillis(result sandbox.TestResult) int {
	if result.Duration <= 0 {
		return 0
	}
	return int(result.Duration.Milliseconds())
}

func tokensOf(output string) []string {
	fields := strings.Fields(output)
	if len(fields) > maxCompareTokens {
		return fields[:maxCompareTokens]
	}
	return fields
}

func sameTokens(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
