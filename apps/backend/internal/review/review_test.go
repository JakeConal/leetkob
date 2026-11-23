package review_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JakeConal/leetkob/backend/internal/review"
)

func TestSampleSizeIsCeilTenPercent(t *testing.T) {
	cases := map[int]int{
		-1: 0, 0: 0, 1: 1, 2: 1, 9: 1, 10: 1, 11: 2, 19: 2, 20: 2, 21: 3, 100: 10, 101: 11,
	}
	for pairs, want := range cases {
		if got := review.SampleSize(pairs); got != want {
			t.Fatalf("SampleSize(%d) = %d, want %d", pairs, got, want)
		}
	}
}

func TestSampleIDsIsStableButNotOrderDependent(t *testing.T) {
	passIDs := []string{
		"f1000000-0000-4000-8000-000000000001",
		"f1000000-0000-4000-8000-000000000002",
		"f1000000-0000-4000-8000-000000000003",
		"f1000000-0000-4000-8000-000000000004",
		"f1000000-0000-4000-8000-000000000005",
		"f1000000-0000-4000-8000-000000000006",
		"f1000000-0000-4000-8000-000000000007",
		"f1000000-0000-4000-8000-000000000008",
		"f1000000-0000-4000-8000-000000000009",
		"f1000000-0000-4000-8000-00000000000a",
		"f1000000-0000-4000-8000-00000000000b",
	}
	first := review.SampleIDs(passIDs)
	if len(first) != 2 {
		t.Fatalf("11 pass pairs sampled %d, want 2", len(first))
	}
	for attempt := 0; attempt < 20; attempt++ {
		again := review.SampleIDs(passIDs)
		if !equalSets(first, again) {
			t.Fatalf("sample redrawn: %v -> %v", first, again)
		}
	}
	reversed := make([]string, len(passIDs))
	for i := range passIDs {
		reversed[i] = passIDs[len(passIDs)-1-i]
	}
	if !equalSets(first, review.SampleIDs(reversed)) {
		t.Fatal("sample depends on input order, not the stable finding rank")
	}
	if len(review.SampleIDs(nil)) != 0 {
		t.Fatal("empty pass set must sample nothing")
	}
}

func TestSampleSpreadsAcrossThePassSet(t *testing.T) {
	// A per-finding hash rank must not simply take the first/N-th IDs: across
	// independent prefixes the sampled member varies, so the sample is random
	// with respect to marking order.
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		ids := make([]string, 0, 10)
		for j := 0; j < 10; j++ {
			ids = append(ids, fmt.Sprintf("finding-%03d-%02d", i, j))
		}
		selected := review.SampleIDs(ids)
		if len(selected) != 1 {
			t.Fatalf("10 pass pairs sampled %d, want 1", len(selected))
		}
		seen[selected[0]] = true
	}
	if len(seen) < 20 {
		t.Fatalf("sample barely varied across pass sets: %d distinct", len(seen))
	}
}

func TestRequiredReviewObligations(t *testing.T) {
	cases := []struct {
		name    string
		finding review.Finding
		want    bool
	}{
		{name: "violation", finding: review.Finding{CheckerVerdict: review.VerdictViolation}, want: true},
		{name: "unsure", finding: review.Finding{CheckerVerdict: review.VerdictUnsure}, want: true},
		{name: "sampled pass", finding: review.Finding{CheckerVerdict: review.VerdictPass, Sampled: true}, want: true},
		{name: "unsampled pass", finding: review.Finding{CheckerVerdict: review.VerdictPass}, want: false},
	}
	for _, test := range cases {
		if got := test.finding.RequiredReview(); got != test.want {
			t.Fatalf("%s: RequiredReview = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestValidateDecisionRequest(t *testing.T) {
	evidence := &review.Evidence{Line: 12, Text: "stable_sort(a, a + n);"}
	cases := []struct {
		name     string
		verdict  string
		reason   string
		evidence *review.Evidence
		valid    bool
	}{
		{name: "explicit pass", verdict: "pass", reason: "matches the constraint", valid: true},
		{name: "explicit violation", verdict: "violation", reason: "banned call", evidence: evidence, valid: true},
		{name: "missing verdict", verdict: "", reason: "x", valid: false},
		{name: "unsure is not a decision", verdict: "unsure", reason: "x", valid: false},
		{name: "blank reason", verdict: "pass", reason: "   ", valid: false},
		{name: "max reason", verdict: "pass", reason: strings.Repeat("r", review.MaxReasonChars), valid: true},
		{name: "oversized reason", verdict: "pass", reason: strings.Repeat("r", review.MaxReasonChars+1), valid: false},
		{name: "violation needs evidence", verdict: "violation", reason: "banned call", valid: false},
		{name: "zero line", verdict: "violation", reason: "x", evidence: &review.Evidence{Line: 0, Text: "x"}, valid: false},
		{name: "empty text", verdict: "violation", reason: "x", evidence: &review.Evidence{Line: 1, Text: ""}, valid: false},
		{name: "oversized text", verdict: "pass", reason: "x",
			evidence: &review.Evidence{Line: 1, Text: strings.Repeat("t", review.MaxEvidenceText+1)}, valid: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := review.ValidateDecisionRequest(test.verdict, test.reason, test.evidence)
			if test.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.valid {
				if err == nil {
					t.Fatal("expected a validation error")
				}
				var validation *review.ValidationError
				if !asValidation(err, &validation) {
					t.Fatalf("error %v is not a ValidationError", err)
				}
			}
		})
	}
}

func TestCanonicalEvidenceMatchesOriginalLine(t *testing.T) {
	source := "int main() {\r\n    stable_sort(a, a + n);\r\n    return 0;\r\n}"
	canonical, err := review.CanonicalEvidence(source, review.Evidence{Line: 2, Text: "    stable_sort(a, a + n);"})
	if err != nil {
		t.Fatalf("matching evidence rejected: %v", err)
	}
	if canonical.Text != "    stable_sort(a, a + n);" || canonical.Line != 2 {
		t.Fatalf("canonical evidence = %+v", canonical)
	}

	bad := []review.Evidence{
		{Line: 2, Text: "stable_sort(a, a + n);"},      // loses the leading indent
		{Line: 2, Text: "    stable_sort(a, a + n); "}, // trailing space differs
		{Line: 9, Text: "return 0;"},                   // no such line
		{Line: 0, Text: "int main() {"},
	}
	for _, evidence := range bad {
		if _, err := review.CanonicalEvidence(source, evidence); err == nil {
			t.Fatalf("evidence %+v was accepted", evidence)
		}
	}
}

func TestMarshalEvidenceKeepsNilDistinct(t *testing.T) {
	encoded, err := review.MarshalEvidence(nil)
	if err != nil || encoded != nil {
		t.Fatalf("nil evidence = %q, %v", encoded, err)
	}
	encoded, err = review.MarshalEvidence(&review.Evidence{Line: 1, Text: "x"})
	if err != nil || string(encoded) != `{"line":1,"text":"x"}` {
		t.Fatalf("evidence json = %s, %v", encoded, err)
	}
}

func equalSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func asValidation(err error, target **review.ValidationError) bool {
	validation, ok := err.(*review.ValidationError)
	if ok {
		*target = validation
	}
	return ok
}
