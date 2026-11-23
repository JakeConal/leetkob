package review

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalid means the decision body is not an explicit pass or
	// violation with a usable reason and, for violation, original evidence.
	ErrInvalid = errors.New("review: invalid decision")
	// ErrNotFound means the finding is not in the active set for that lab.
	ErrNotFound = errors.New("review: not found")
	// ErrNotAssigned means the caller is not the current TA for the lab.
	ErrNotAssigned = errors.New("review: not assigned")
	// ErrBadCursor means the pagination cursor cannot be read.
	ErrBadCursor = errors.New("review: bad cursor")
	errDeduction = errors.New("review: deduction out of range")
)

// StaleRevision means the decision named a finding revision that is no
// longer current. Current is the revision stored now.
type StaleRevision struct {
	Expected int
	Current  int
}

func (s *StaleRevision) Error() string {
	if s == nil {
		return "review: stale revision"
	}
	return fmt.Sprintf("review: stale revision %d, current %d", s.Expected, s.Current)
}

// ApplyDecision checks one explicit decision against the current finding
// and returns the next finding snapshot. Checker verdict, source kind,
// evidence, unsure cause, and deduction are copied unchanged.
func ApplyDecision(current Finding, req DecisionRequest) (Finding, error) {
	if req.ExpectedRevision < 1 || req.ActorID == "" {
		return Finding{}, ErrInvalid
	}
	if req.ExpectedRevision != current.Revision {
		return Finding{}, &StaleRevision{Expected: req.ExpectedRevision, Current: current.Revision}
	}
	if req.Verdict != "pass" && req.Verdict != "violation" {
		return Finding{}, ErrInvalid
	}
	if strings.TrimSpace(req.Reason) == "" || len([]rune(req.Reason)) > 2000 {
		return Finding{}, ErrInvalid
	}
	var cited *Evidence
	if req.Evidence != nil {
		if !lineMatches(current.Source, req.Evidence.Line, req.Evidence.Text) {
			return Finding{}, ErrInvalid
		}
		copy := *req.Evidence
		cited = &copy
	}
	if req.Verdict == "violation" && cited == nil {
		return Finding{}, ErrInvalid
	}
	when := req.Now
	if when.IsZero() {
		when = time.Now()
	}
	next := cloneFinding(current)
	next.Revision = current.Revision + 1
	decision := &TADecision{
		Verdict:   req.Verdict,
		Reason:    req.Reason,
		Revision:  next.Revision,
		DecidedBy: req.ActorID,
		DecidedAt: when.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if req.Verdict == "violation" {
		decision.Evidence = cited
	}
	next.Decision = decision
	if next.SourceKind != current.SourceKind || next.CheckerVerdict != current.CheckerVerdict ||
		next.UnsureCause != current.UnsureCause || next.Deduction != current.Deduction ||
		!sameEvidence(next.Evidence, current.Evidence) {
		return Finding{}, errors.New("review: checker provenance changed")
	}
	return next, nil
}

func lineMatches(source string, line int, text string) bool {
	if line < 1 || text == "" || len(text) > 4096 {
		return false
	}
	parts := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if line > len(parts) {
		return false
	}
	got := strings.TrimRight(parts[line-1], "\r")
	return got != "" && got == text
}

func sameEvidence(left, right []Evidence) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func cloneFinding(in Finding) Finding {
	out := in
	if in.Evidence == nil {
		out.Evidence = []Evidence{}
	} else {
		out.Evidence = append([]Evidence(nil), in.Evidence...)
	}
	if in.Decision != nil {
		decision := *in.Decision
		if in.Decision.Evidence != nil {
			evidence := *in.Decision.Evidence
			decision.Evidence = &evidence
		}
		out.Decision = &decision
	}
	return out
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func twoDigits(n int) string {
	if n < 10 {
		return fmt.Sprintf("0%d", n)
	}
	return fmt.Sprintf("%d", n)
}
