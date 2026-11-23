// Package checking runs hard rules, then bounded semantic checks, for the
// current counted submission whose judge verdict is AC. A non-AC submission
// writes no findings and does not call the model. A stale revision is not
// stored as the active finding set.
package checking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
	"github.com/JakeConal/leetkob/backend/internal/review"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

const (
	// SourceHardRule is a finding decided without a model call.
	SourceHardRule = "hard-rule"
	// SourceSemantic is a finding produced by the semantic checker.
	SourceSemantic = "semantic"
)

// Verdict is the checker outcome stored for one constraint.
type Verdict string

const (
	VerdictPass      Verdict = "pass"
	VerdictViolation Verdict = "violation"
	VerdictUnsure    Verdict = "unsure"
)

var (
	// ErrNotFound means the submission id is not in the store.
	ErrNotFound = errors.New("checking: submission not found")
	// ErrStale means the revision tuple is no longer the counted AC submission.
	ErrStale = errors.New("checking: stale revision")
	// ErrAlreadyChecked means this revision already has a finding set.
	ErrAlreadyChecked = errors.New("checking: findings already stored")
)

// Evidence is one original source line.
type Evidence struct {
	Line int
	Text string
}

// Constraint is one lab constraint at the submission's revision.
type Constraint struct {
	ID                  string
	Text                string
	DeductionHundredths int
	Position            int
}

// Submission is the counted work a check reads. SourceSHA256 is the hex
// digest stored with the submission.
type Submission struct {
	ID                 string
	LabID              string
	AuthorUserID       string
	Source             string
	SourceSHA256       string
	JudgeVerdict       string
	Generation         int
	TestRevision       int
	ConstraintRevision int
	Counted            bool
	Checked            bool
	Statement          string
	RedactionTerms     []string
	Constraints        []Constraint
}

// Finding is one constraint result ready to store. Revision starts at 1.
// DeductionHundredths is the TA configuration, never a model score.
type Finding struct {
	ConstraintID        string
	SourceKind          string
	Verdict             Verdict
	Evidence            []Evidence
	UnsureCause         string
	DeductionHundredths int
	RequiredReview      bool
	Revision            int
}

// Outcome is the result of one check attempt.
type Outcome struct {
	Findings   []Finding
	ModelCalls int
	Persisted  bool
	Stale      bool
}

// AnalyzeFunc reads one submission and returns an analyzer-result document.
// Tests supply a fake. Production passes the native analyzer's stdout.
type AnalyzeFunc func(ctx context.Context, source string) ([]byte, error)

// Store loads a submission and commits a finding set only when that
// submission is still the counted AC work at the same revision.
type Store interface {
	Load(ctx context.Context, submissionID string) (Submission, error)
	Save(ctx context.Context, sub Submission, findings []Finding) error
}

// Service is the check pipeline. Checker and Price are used only for
// constraints the hard rules do not decide.
type Service struct {
	Store   Store
	Analyze AnalyzeFunc
	Checker *semantic.Checker
	Price   semantic.Price
}

// SourceHash is the hex SHA-256 of the submitted source bytes.
func SourceHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// Run checks one submission. A submission that is not the current counted
// AC work returns an empty outcome and does not call the analyzer or the model.
func (s *Service) Run(ctx context.Context, submissionID string) (Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	if s == nil || s.Store == nil {
		return Outcome{}, errors.New("checking: store is required")
	}
	sub, err := s.Store.Load(ctx, submissionID)
	if err != nil {
		return Outcome{}, err
	}
	if !sub.Counted || sub.JudgeVerdict != "AC" {
		return Outcome{}, nil
	}
	if sub.Checked {
		return Outcome{}, nil
	}
	if SourceHash(sub.Source) != sub.SourceSHA256 {
		return Outcome{}, errors.New("checking: source hash does not match the stored submission")
	}
	if len(sub.Constraints) == 0 {
		return Outcome{}, errors.New("checking: the lab revision has no constraints")
	}
	if s.Analyze == nil {
		return Outcome{}, errors.New("checking: analyzer is not configured")
	}
	raw, err := s.Analyze(ctx, sub.Source)
	if err != nil {
		return Outcome{}, err
	}
	decisions := make(map[string]hardrule.Decision, len(sub.Constraints))
	var pending []semantic.Constraint
	for _, constraint := range sub.Constraints {
		decision := hardrule.Evaluate(constraint.Text, sub.Source, raw)
		if decision.HardRule {
			decisions[constraint.ID] = decision
			continue
		}
		pending = append(pending, semantic.Constraint{ID: constraint.ID, Text: constraint.Text})
	}
	var calls int
	semanticByID := map[string]semantic.Finding{}
	if len(pending) > 0 {
		if s.Checker == nil {
			return Outcome{}, errors.New("checking: semantic checker is not configured")
		}
		result, checkErr := s.Checker.Check(ctx, semantic.Input{
			LabID:          sub.LabID,
			Statement:      sub.Statement,
			Constraints:    pending,
			Source:         sub.Source,
			RedactionTerms: sub.RedactionTerms,
			Price:          s.Price,
		})
		calls = result.ModelCalls
		if checkErr != nil {
			return Outcome{ModelCalls: calls}, checkErr
		}
		for _, finding := range result.Findings {
			semanticByID[finding.ConstraintID] = finding
		}
	}
	findings := make([]Finding, 0, len(sub.Constraints))
	for _, constraint := range sub.Constraints {
		if decision, ok := decisions[constraint.ID]; ok {
			findings = append(findings, fromHard(constraint, decision))
			continue
		}
		got, ok := semanticByID[constraint.ID]
		if !ok {
			return Outcome{ModelCalls: calls}, errors.New("checking: semantic result missed a constraint")
		}
		findings = append(findings, fromSemantic(constraint, got))
	}
	markObligations(sub.ID, findings)
	if err := s.Store.Save(ctx, sub, findings); err != nil {
		if errors.Is(err, ErrStale) {
			return Outcome{ModelCalls: calls, Stale: true}, ErrStale
		}
		if errors.Is(err, ErrAlreadyChecked) {
			return Outcome{ModelCalls: calls}, nil
		}
		return Outcome{ModelCalls: calls}, err
	}
	return Outcome{Findings: findings, ModelCalls: calls, Persisted: true}, nil
}

func fromHard(constraint Constraint, decision hardrule.Decision) Finding {
	finding := Finding{
		ConstraintID:        constraint.ID,
		SourceKind:          SourceHardRule,
		Verdict:             Verdict(decision.Verdict),
		DeductionHundredths: constraint.DeductionHundredths,
		Revision:            1,
		Evidence:            []Evidence{},
	}
	if decision.Verdict == schema.VerdictUnsure {
		finding.UnsureCause = decision.Reason
	}
	for _, item := range decision.Evidence {
		finding.Evidence = append(finding.Evidence, Evidence{Line: item.Line, Text: item.Text})
	}
	return finding
}

func fromSemantic(constraint Constraint, got semantic.Finding) Finding {
	finding := Finding{
		ConstraintID:        constraint.ID,
		SourceKind:          SourceSemantic,
		Verdict:             Verdict(got.Verdict),
		DeductionHundredths: constraint.DeductionHundredths,
		Revision:            1,
		Evidence:            make([]Evidence, 0, len(got.Evidence)),
	}
	if got.Verdict == semantic.VerdictUnsure {
		finding.UnsureCause = string(got.UnsureCause)
	}
	for _, item := range got.Evidence {
		finding.Evidence = append(finding.Evidence, Evidence{Line: item.Line, Text: item.Text})
	}
	return finding
}

func markObligations(submissionID string, findings []Finding) {
	pairs := make([]review.Pair, len(findings))
	for i, finding := range findings {
		pairs[i] = review.Pair{ConstraintID: finding.ConstraintID, Verdict: string(finding.Verdict)}
	}
	required := review.Obligations(submissionID, pairs)
	for i := range findings {
		findings[i].RequiredReview = required[findings[i].ConstraintID]
	}
}
