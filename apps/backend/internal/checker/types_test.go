package checker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPredictionJSONMatchesContractShape(t *testing.T) {
	prediction := Prediction{
		Solution:   "solution-01",
		Constraint: "C1",
		Verdict:    VerdictPass,
	}
	raw, err := json.Marshal(prediction)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"evidence":null`) {
		t.Fatalf("pass prediction encoded null evidence: %s", raw)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantKeys := []string{"solution", "constraint", "verdict", "evidence"}
	if len(decoded) != len(wantKeys) {
		t.Fatalf("keys = %v, want exactly %v", decoded, wantKeys)
	}
	for _, key := range wantKeys {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
	var evidence []Evidence
	if err := json.Unmarshal(decoded["evidence"], &evidence); err != nil {
		t.Fatalf("evidence is not an array: %v", err)
	}
	if len(evidence) != 0 {
		t.Fatalf("evidence = %v, want empty", evidence)
	}
}

func TestPredictionViolationJSONCarriesOriginalLine(t *testing.T) {
	prediction := Prediction{
		Solution:   "solution-01",
		Constraint: "C1",
		Verdict:    VerdictViolation,
		Evidence:   []Evidence{{Line: 3, Text: "  std::sort(v.begin(), v.end());"}},
	}
	if err := prediction.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	raw, err := json.Marshal(prediction)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"solution":"solution-01","constraint":"C1","verdict":"violation","evidence":[{"line":3,"text":"  std::sort(v.begin(), v.end());"}]}`
	if string(raw) != want {
		t.Fatalf("JSON = %s, want %s", raw, want)
	}
}

func TestPredictionValidateRejectsOutOfContractRows(t *testing.T) {
	longText := strings.Repeat("x", maxEvidenceTextLength+1)
	tests := []struct {
		name       string
		prediction Prediction
	}{
		{
			name:       "empty solution",
			prediction: Prediction{Constraint: "C1", Verdict: VerdictPass},
		},
		{
			name:       "empty constraint",
			prediction: Prediction{Solution: "s", Verdict: VerdictPass},
		},
		{
			name:       "unknown verdict",
			prediction: Prediction{Solution: "s", Constraint: "C1", Verdict: Verdict("maybe")},
		},
		{
			name:       "violation without evidence",
			prediction: Prediction{Solution: "s", Constraint: "C1", Verdict: VerdictViolation},
		},
		{
			name: "zero evidence line",
			prediction: Prediction{
				Solution: "s", Constraint: "C1", Verdict: VerdictViolation,
				Evidence: []Evidence{{Line: 0, Text: "std::sort(v);"}},
			},
		},
		{
			name: "empty evidence text",
			prediction: Prediction{
				Solution: "s", Constraint: "C1", Verdict: VerdictViolation,
				Evidence: []Evidence{{Line: 1}},
			},
		},
		{
			name: "oversized evidence text",
			prediction: Prediction{
				Solution: "s", Constraint: "C1", Verdict: VerdictViolation,
				Evidence: []Evidence{{Line: 1, Text: longText}},
			},
		},
		{
			name:       "oversized solution id",
			prediction: Prediction{Solution: strings.Repeat("s", maxPredictionLabelLength+1), Constraint: "C1", Verdict: VerdictUnsure},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.prediction.Validate()
			if !errors.Is(err, ErrInvalidPrediction) {
				t.Fatalf("Validate error = %v, want ErrInvalidPrediction", err)
			}
		})
	}
}

func TestPredictionValidateAcceptsEmptyEvidenceForUnsure(t *testing.T) {
	prediction := Prediction{Solution: "s", Constraint: "C1", Verdict: VerdictUnsure}
	if err := prediction.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	raw, err := json.Marshal(prediction)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"evidence":[]`) {
		t.Fatalf("unsure evidence = %s, want []", raw)
	}
}

func TestVerdictValid(t *testing.T) {
	for _, verdict := range []Verdict{VerdictPass, VerdictViolation, VerdictUnsure} {
		if !verdict.Valid() {
			t.Fatalf("%q should be valid", verdict)
		}
	}
	for _, verdict := range []Verdict{"", "PASS", "maybe"} {
		if verdict.Valid() {
			t.Fatalf("%q should be invalid", verdict)
		}
	}
}

func TestIsBanListConstraint(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Do not use `std::sort`.", true},
		{"Do not call `std::sort`.", true},
		{"  Do not use `std::sort`.", true},
		{"do not use `std::sort`.", true},
		{"Implement merge sort.", false},
		{"Do not `std::sort`.", false},
		{"", false},
	}
	for _, test := range tests {
		if got := IsBanListConstraint(test.text); got != test.want {
			t.Fatalf("IsBanListConstraint(%q) = %v, want %v", test.text, got, test.want)
		}
	}
}

func TestBannedNamesExtractsGenericBacktickedNames(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "single call",
			text: "Do not call `std::sort`.",
			want: []string{"std::sort"},
		},
		{
			name: "multiple names",
			text: "Do not use `sort` or `std::stable_sort` in the solution.",
			want: []string{"sort", "std::stable_sort"},
		},
		{
			name: "leading whitespace",
			text: "   Do not use `std::ranges::sort`.",
			want: []string{"std::ranges::sort"},
		},
		{
			name: "duplicates collapse",
			text: "Do not call `sort`, `sort`, and `std::sort`.",
			want: []string{"sort", "std::sort"},
		},
		{
			name: "empty backticks ignored",
			text: "Do not use `` or `std::sort`.",
			want: []string{"std::sort"},
		},
		{
			name: "unclosed backtick ignored",
			text: "Do not use `std::sort.",
			want: nil,
		},
		{
			name: "not a ban list",
			text: "Implement `std::sort` from scratch.",
			want: nil,
		},
		{
			name: "ban list without backticks",
			text: "Do not use std::sort.",
			want: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BannedNames(test.text)
			if len(got) != len(test.want) {
				t.Fatalf("BannedNames(%q) = %v, want %v", test.text, got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("BannedNames(%q) = %v, want %v", test.text, got, test.want)
				}
			}
		})
	}
}

func TestBannedNamesAreBounded(t *testing.T) {
	var text strings.Builder
	text.WriteString("Do not use ")
	for i := 0; i < MaxBannedNamesPerConstraint+5; i++ {
		fmt.Fprintf(&text, "`name%d` ", i)
	}
	names := BannedNames(text.String())
	if len(names) != MaxBannedNamesPerConstraint {
		t.Fatalf("len(names) = %d, want %d", len(names), MaxBannedNamesPerConstraint)
	}
}
