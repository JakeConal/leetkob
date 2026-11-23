package review

import (
	"fmt"
	"testing"
)

func TestObligationsCeilTenPercentAndStayStable(t *testing.T) {
	cases := []struct {
		passes int
		want   int
	}{
		{passes: 1, want: 1},
		{passes: 10, want: 1},
		{passes: 11, want: 2},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d passes", tc.passes), func(t *testing.T) {
			pairs := passPairs(tc.passes)
			pairs = append(pairs,
				Pair{ConstraintID: "violation", Verdict: "violation"},
				Pair{ConstraintID: "unsure", Verdict: "unsure"},
			)
			first := Obligations("submission-1", pairs)
			second := Obligations("submission-1", pairs)
			if !first["violation"] || !first["unsure"] {
				t.Fatalf("violation and unsure must be required: %+v", first)
			}
			passRequired := 0
			for _, pair := range pairs {
				if first[pair.ConstraintID] != second[pair.ConstraintID] {
					t.Fatalf("sample for %s changed between calls", pair.ConstraintID)
				}
				if pair.Verdict == "pass" && first[pair.ConstraintID] {
					passRequired++
				}
			}
			if passRequired != tc.want {
				t.Fatalf("required passes = %d, want ceil(10%%) = %d", passRequired, tc.want)
			}
		})
	}
}

func TestObligationsWithNoPassesDoesNotPanic(t *testing.T) {
	got := Obligations("submission-1", []Pair{{ConstraintID: "u", Verdict: "unsure"}})
	if !got["u"] {
		t.Fatal("unsure must be required")
	}
}

func passPairs(n int) []Pair {
	pairs := make([]Pair, n)
	for i := 0; i < n; i++ {
		pairs[i] = Pair{ConstraintID: fmt.Sprintf("p%02d", i), Verdict: "pass"}
	}
	return pairs
}
