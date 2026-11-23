package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrSubmissionNotFound is returned when a submission row does not exist.
// The HTTP boundary conceals it exactly like an inaccessible submission.
var ErrSubmissionNotFound = errors.New("postgres: submission not found")

// SubmissionTestResult is one stored per-test verdict. It never carries
// hidden input, expected output or checker evidence.
type SubmissionTestResult struct {
	TestID  string
	Verdict string
	TimeMs  int
}

// SubmissionView is the safe read projection of one submission. ClassID is
// used only for current authorization; it is not serialized to students.
type SubmissionView struct {
	ID         string
	LabID      string
	StudentID  string
	ClassID    string
	State      string
	ReceivedAt time.Time
	Verdict    *string
	Tests      []SubmissionTestResult
}

// GetSubmissionForRead reads one safe submission with the ownership fields
// the HTTP boundary needs for current authorization. Hidden lab data is
// never selected.
func (s *Store) GetSubmissionForRead(ctx context.Context, submissionID string) (SubmissionView, error) {
	var view SubmissionView
	err := s.pool.QueryRow(ctx, `
		SELECT s.id, s.lab_id, s.student_id, l.class_id, s.state, s.received_at, s.verdict
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.id = $1`, submissionID,
	).Scan(&view.ID, &view.LabID, &view.StudentID, &view.ClassID, &view.State,
		&view.ReceivedAt, &view.Verdict)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmissionView{}, ErrSubmissionNotFound
		}
		return SubmissionView{}, fmt.Errorf("postgres: read submission: %w", err)
	}
	views, err := s.loadTestResultsFor(ctx, []SubmissionView{view})
	if err != nil {
		return SubmissionView{}, err
	}
	return views[0], nil
}

// ListSubmissionsForLab returns safe submissions of one lab in newest-first
// receipt order. The HTTP boundary has already checked that the caller is
// the assigned TA of the lab's class.
func (s *Store) ListSubmissionsForLab(ctx context.Context, labID string, limit, offset int) ([]SubmissionView, error) {
	return s.listSubmissions(ctx, `
		SELECT s.id, s.lab_id, s.student_id, l.class_id, s.state, s.received_at, s.verdict
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.lab_id = $1
		ORDER BY s.received_at DESC, s.receipt_seq DESC, s.id
		LIMIT $2 OFFSET $3`, labID, limit, offset)
}

// ListSubmissionsForStudent returns only the named student's safe
// submissions for one lab. The owner filter is applied before pagination.
func (s *Store) ListSubmissionsForStudent(ctx context.Context, labID, studentID string, limit, offset int) ([]SubmissionView, error) {
	return s.listSubmissions(ctx, `
		SELECT s.id, s.lab_id, s.student_id, l.class_id, s.state, s.received_at, s.verdict
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.lab_id = $1 AND s.student_id = $2
		ORDER BY s.received_at DESC, s.receipt_seq DESC, s.id
		LIMIT $3 OFFSET $4`, labID, studentID, limit, offset)
}

// LabClassID reads the class that owns a lab, or ErrLabNotFound. Callers
// conceal an absent lab and another class's lab identically.
func (s *Store) LabClassID(ctx context.Context, labID string) (string, error) {
	var classID string
	err := s.pool.QueryRow(ctx, `SELECT class_id FROM labs WHERE id = $1`, labID).Scan(&classID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrLabNotFound
		}
		return "", fmt.Errorf("postgres: read lab class: %w", err)
	}
	return classID, nil
}

func (s *Store) listSubmissions(ctx context.Context, query string, args ...any) ([]SubmissionView, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list submissions: %w", err)
	}
	defer rows.Close()

	views := make([]SubmissionView, 0, 8)
	for rows.Next() {
		var view SubmissionView
		if err := rows.Scan(&view.ID, &view.LabID, &view.StudentID, &view.ClassID,
			&view.State, &view.ReceivedAt, &view.Verdict); err != nil {
			return nil, fmt.Errorf("postgres: scan submission: %w", err)
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate submissions: %w", err)
	}
	return s.loadTestResultsFor(ctx, views)
}

// loadTestResults attaches ordered per-test results to a page of
// submissions using one bounded query.
func (s *Store) loadTestResultsFor(ctx context.Context, views []SubmissionView) ([]SubmissionView, error) {
	if len(views) == 0 {
		return views, nil
	}
	ids := make([]string, 0, len(views))
	index := make(map[string]int, len(views))
	for position, view := range views {
		ids = append(ids, view.ID)
		index[view.ID] = position
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.submission_id, r.test_id, r.verdict, r.time_ms
		FROM submission_test_results r
		JOIN submissions s ON s.id = r.submission_id
		JOIN lab_tests t ON t.lab_id = s.lab_id AND t.test_id = r.test_id
		WHERE r.submission_id = ANY($1::text[]::uuid[])
		ORDER BY r.submission_id, t.position, r.test_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("postgres: read test results: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var submissionID string
		var result SubmissionTestResult
		if err := rows.Scan(&submissionID, &result.TestID, &result.Verdict, &result.TimeMs); err != nil {
			return nil, fmt.Errorf("postgres: scan test result: %w", err)
		}
		position, ok := index[submissionID]
		if !ok {
			continue
		}
		views[position].Tests = append(views[position].Tests, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate test results: %w", err)
	}
	return views, nil
}
