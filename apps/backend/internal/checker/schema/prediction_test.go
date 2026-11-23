package schema

import "testing"

func TestValidatePredictionAcceptsEmptyEvidence(t *testing.T) {
	err := ValidatePrediction(Prediction{
		Solution:   "fixture/s01",
		Constraint: "c1",
		Verdict:    VerdictUnsure,
		Evidence:   []Evidence{},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatePredictionRejectsBadEvidence(t *testing.T) {
	err := ValidatePrediction(Prediction{
		Solution:   "fixture/s01",
		Constraint: "c1",
		Verdict:    VerdictViolation,
		Evidence:   []Evidence{{Line: 0, Text: "x"}},
	})
	if err == nil {
		t.Fatal("line 0 must be rejected")
	}
}
