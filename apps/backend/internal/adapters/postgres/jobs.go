package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JakeConal/leetkob/backend/internal/checking"
	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/judge"
	"github.com/JakeConal/leetkob/backend/internal/submit"
)

const createSubmissionOperation = "createSubmission"

var (
	_ submit.Repository = (*Store)(nil)
	_ judge.Queue       = (*Store)(nil)
	_ judge.Loader      = (*Store)(nil)
)

type labAccess struct {
	found              bool
	classID            string
	role               string
	enrolled           bool
	opensAt            time.Time
	deadline           time.Time
	testRevision       int
	constraintRevision int
	cpuSeconds         int
	memoryMiB          int
}

// BeginAdmission records a lease for an enrolled student before the body is read.
func (s *Store) BeginAdmission(ctx context.Context, userID, labID string, now, expires time.Time) (submit.Admission, error) {
	if !expires.After(now) {
		return submit.Admission{}, errors.New("postgres: admission lease must be in the future")
	}
	var out submit.Admission
	err := s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		access, err := readAccess(ctx, tx, userID, labID, false)
		if err != nil {
			return err
		}
		if !access.found || !access.enrolled {
			return submit.ErrNotFound
		}
		if access.role != "student" {
			return submit.ErrForbidden
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO admission_leases (user_id, lab_id, expires_at)
			VALUES ($1, $2, $3)
			RETURNING id::text`, userID, labID, expires).Scan(&id); err != nil {
			return fmt.Errorf("postgres: admission: %w", err)
		}
		out = submit.Admission{ID: id, UserID: userID, LabID: labID, ExpiresAt: expires}
		return nil
	})
	return out, err
}

// AbortAdmission closes a lease when the body is rejected before acceptance.
func (s *Store) AbortAdmission(ctx context.Context, leaseID string, now time.Time) error {
	if leaseID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE admission_leases
		SET consumed_at = $2
		WHERE id = $1 AND consumed_at IS NULL`, leaseID, now)
	if err != nil {
		return fmt.Errorf("postgres: abort admission: %w", err)
	}
	return nil
}

// Accept commits source, counted selection, the judge job, and the replay
// row together. A successful body is returned only after that commit.
func (s *Store) Accept(ctx context.Context, in submit.AcceptIn) (submit.AcceptOut, error) {
	var out submit.AcceptOut
	var appErr error
	err := s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		out, appErr = acceptInTx(ctx, tx, in)
		if appErr != nil && !expectedSubmitErr(appErr) {
			return appErr
		}
		return nil
	})
	if err != nil {
		return submit.AcceptOut{}, err
	}
	return out, appErr
}

func expectedSubmitErr(err error) bool {
	var closed submit.WindowClosed
	var mismatch submit.IdempotencyMismatch
	return errors.Is(err, submit.ErrNotFound) ||
		errors.Is(err, submit.ErrForbidden) ||
		errors.Is(err, submit.ErrInvalid) ||
		errors.Is(err, submit.ErrLeaseExpired) ||
		errors.As(err, &closed) ||
		errors.As(err, &mismatch)
}

func acceptInTx(ctx context.Context, tx *Tx, in submit.AcceptIn) (submit.AcceptOut, error) {
	if !submit.ValidUUID(in.UserID) || !submit.ValidUUID(in.LabID) || !submit.ValidUUID(in.LeaseID) || !submit.ValidIdempotencyKey(in.IdempotencyKey) {
		return submit.AcceptOut{}, submit.ErrInvalid
	}
	if len(in.Source) < 1 || len(in.Source) > submit.MaxSourceBytes || strings.ContainsRune(in.Source, 0) {
		return submit.AcceptOut{}, submit.ErrInvalid
	}
	access, err := readAccess(ctx, tx, in.UserID, in.LabID, true)
	if err != nil {
		return submit.AcceptOut{}, err
	}
	var leaseUser, leaseLab string
	var leaseExpires time.Time
	var consumed *time.Time
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, lab_id::text, expires_at, consumed_at
		FROM admission_leases
		WHERE id = $1
		FOR UPDATE`, in.LeaseID).Scan(&leaseUser, &leaseLab, &leaseExpires, &consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return submit.AcceptOut{}, submit.ErrNotFound
	}
	if err != nil {
		return submit.AcceptOut{}, fmt.Errorf("postgres: lock admission: %w", err)
	}
	if leaseUser != in.UserID || leaseLab != in.LabID || consumed != nil {
		return submit.AcceptOut{}, submit.ErrNotFound
	}
	if !access.found || !access.enrolled {
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		return submit.AcceptOut{}, submit.ErrNotFound
	}
	if access.role != "student" {
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		return submit.AcceptOut{}, submit.ErrForbidden
	}

	sum := sha256.Sum256([]byte(in.Source))
	fingerprint := hex.EncodeToString(sum[:])
	var storedFP string
	var storedStatus int
	var storedBody []byte
	err = tx.QueryRow(ctx, `
		SELECT request_fingerprint, status_code, response_body
		FROM replays
		WHERE actor_user_id = $1 AND operation = $2 AND resource = $3 AND idempotency_key = $4
		FOR UPDATE`, in.UserID, createSubmissionOperation, in.LabID, in.IdempotencyKey).Scan(&storedFP, &storedStatus, &storedBody)
	switch {
	case err == nil:
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		if storedFP == fingerprint {
			return submit.AcceptOut{Status: storedStatus, Body: storedBody}, nil
		}
		return submit.AcceptOut{}, submit.IdempotencyMismatch{}
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return submit.AcceptOut{}, fmt.Errorf("postgres: lock replay: %w", err)
	}

	if !in.ReceivedAt.Before(leaseExpires) {
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		return submit.AcceptOut{}, submit.ErrLeaseExpired
	}
	if in.ReceivedAt.Before(access.opensAt) {
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		return submit.AcceptOut{}, submit.ErrNotFound
	}
	// Equality is closed: a body that finishes at the deadline is rejected.
	if !in.ReceivedAt.Before(access.deadline) {
		if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
			return submit.AcceptOut{}, err
		}
		return submit.AcceptOut{}, submit.WindowClosed{Deadline: access.deadline}
	}

	var submissionID string
	var serverSeq int64
	err = tx.QueryRow(ctx, `
		INSERT INTO submissions (
			lab_id, author_user_id, source, source_sha256, state, received_at,
			generation, test_revision, constraint_revision
		) VALUES ($1, $2, $3, $4, 'queued', $5, 1, $6, $7)
		RETURNING id::text, server_seq`,
		in.LabID, in.UserID, in.Source, fingerprint, in.ReceivedAt, access.testRevision, access.constraintRevision,
	).Scan(&submissionID, &serverSeq)
	if err != nil {
		return submit.AcceptOut{}, fmt.Errorf("postgres: insert submission: %w", err)
	}

	generation, won, err := claimCounted(ctx, tx, in.LabID, in.UserID, submissionID, in.ReceivedAt, serverSeq)
	if err != nil {
		return submit.AcceptOut{}, err
	}
	if err := tx.Exec(ctx, `UPDATE submissions SET generation = $1 WHERE id = $2`, generation, submissionID); err != nil {
		return submit.AcceptOut{}, err
	}
	if err := tx.Exec(ctx, `
		INSERT INTO jobs (
			id, submission_id, lab_id, kind, attempt, generation,
			test_revision, constraint_revision, source_sha256, state
		) VALUES (gen_random_uuid(), $1, $2, 'judge', 0, $3, $4, $5, $6, 'pending')`,
		submissionID, in.LabID, generation, access.testRevision, access.constraintRevision, fingerprint); err != nil {
		return submit.AcceptOut{}, fmt.Errorf("postgres: insert judge job: %w", err)
	}
	if won {
		if err := tx.Exec(ctx, `
			UPDATE jobs
			SET state = 'superseded', lease_expires_at = NULL, worker_id = NULL
			WHERE kind = 'check'
			  AND state IN ('pending', 'leased')
			  AND lab_id = $1
			  AND submission_id IN (
			      SELECT id FROM submissions
			      WHERE lab_id = $1 AND author_user_id = $2 AND id <> $3
			  )`, in.LabID, in.UserID, submissionID); err != nil {
			return submit.AcceptOut{}, fmt.Errorf("postgres: supersede check jobs: %w", err)
		}
	}
	body, err := submit.MarshalAccept(submissionID, in.LabID, in.ReceivedAt)
	if err != nil {
		return submit.AcceptOut{}, err
	}
	if err := tx.Exec(ctx, `
		INSERT INTO replays (
			actor_user_id, operation, resource, idempotency_key,
			request_fingerprint, status_code, response_body
		) VALUES ($1, $2, $3, $4, $5, 202, $6::jsonb)`,
		in.UserID, createSubmissionOperation, in.LabID, in.IdempotencyKey, fingerprint, string(body)); err != nil {
		if isIdempotencyUnique(err) {
			return submit.AcceptOut{}, submit.ErrReplayRace
		}
		return submit.AcceptOut{}, fmt.Errorf("postgres: insert replay: %w", err)
	}
	if err := consumeLease(ctx, tx, in.LeaseID, in.ReceivedAt); err != nil {
		return submit.AcceptOut{}, err
	}
	return submit.AcceptOut{Status: 202, Body: body}, nil
}

func claimCounted(ctx context.Context, tx *Tx, labID, userID, submissionID string, received time.Time, serverSeq int64) (int, bool, error) {
	var generation int
	err := tx.QueryRow(ctx, `
		INSERT INTO counted_selections (
			lab_id, author_user_id, submission_id, generation, received_at, server_seq
		) VALUES ($1, $2, $3, 1, $4, $5)
		ON CONFLICT (lab_id, author_user_id) DO UPDATE
		SET submission_id = EXCLUDED.submission_id,
		    generation = counted_selections.generation + 1,
		    received_at = EXCLUDED.received_at,
		    server_seq = EXCLUDED.server_seq
		WHERE (EXCLUDED.received_at, EXCLUDED.server_seq)
		    > (counted_selections.received_at, counted_selections.server_seq)
		RETURNING generation`, labID, userID, submissionID, received, serverSeq).Scan(&generation)
	if err == nil {
		return generation, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("postgres: counted selection: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT generation FROM counted_selections
		WHERE lab_id = $1 AND author_user_id = $2`, labID, userID).Scan(&generation); err != nil {
		return 0, false, fmt.Errorf("postgres: read counted generation: %w", err)
	}
	return generation, false, nil
}

func consumeLease(ctx context.Context, tx *Tx, leaseID string, now time.Time) error {
	tag, err := tx.pg.Exec(ctx, `
		UPDATE admission_leases
		SET consumed_at = $2
		WHERE id = $1 AND consumed_at IS NULL`, leaseID, now)
	if err != nil {
		return fmt.Errorf("postgres: consume admission: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("postgres: admission was not open")
	}
	return nil
}

func isIdempotencyUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "idempotency")
}

// GetSubmission returns one safe submission for its owner or the assigned TA.
func (s *Store) GetSubmission(ctx context.Context, userID, submissionID string, now time.Time) (submit.View, error) {
	var view submit.View
	var author, classID, state string
	var verdict, infra *string
	var opensAt time.Time
	var testRevision, constraintRevision int
	err := s.pool.QueryRow(ctx, `
		SELECT s.id::text, s.lab_id::text, s.state, s.received_at, s.author_user_id::text,
		       s.judge_verdict, s.infrastructure_error, s.test_revision, s.constraint_revision,
		       l.class_id::text, l.opens_at
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.id = $1`, submissionID).Scan(
		&view.ID, &view.LabID, &state, &view.ReceivedAt, &author,
		&verdict, &infra, &testRevision, &constraintRevision, &classID, &opensAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return submit.View{}, submit.ErrNotFound
	}
	if err != nil {
		return submit.View{}, fmt.Errorf("postgres: get submission: %w", err)
	}
	role, enrolled, err := membership(ctx, s.pool, userID, classID)
	if err != nil {
		return submit.View{}, err
	}
	if !enrolled {
		return submit.View{}, submit.ErrNotFound
	}
	switch role {
	case "ta":
	case "student":
		if author != userID || now.Before(opensAt) {
			return submit.View{}, submit.ErrNotFound
		}
	default:
		return submit.View{}, submit.ErrForbidden
	}
	visible, showJudge := visibleState(state, verdict, infra)
	view.State = visible
	if showJudge {
		tests, err := loadTests(ctx, s.pool, view.ID, view.LabID, testRevision, constraintRevision)
		if err != nil {
			return submit.View{}, err
		}
		view.Judge = &submit.JudgeView{Verdict: *verdict, Tests: tests}
	}
	return view, nil
}

// ListSubmissions returns a page the caller is allowed to see.
func (s *Store) ListSubmissions(ctx context.Context, q submit.ListQuery) (submit.Page, error) {
	if q.Limit < 1 || q.Limit > 100 {
		return submit.Page{}, submit.ErrInvalid
	}
	access, err := readAccess(ctx, s.pool, q.UserID, q.LabID, false)
	if err != nil {
		return submit.Page{}, err
	}
	if !access.found || !access.enrolled {
		return submit.Page{}, submit.ErrNotFound
	}
	author := ""
	switch access.role {
	case "ta":
	case "student":
		if q.Now.Before(access.opensAt) {
			return submit.Page{}, submit.ErrNotFound
		}
		author = q.UserID
	default:
		return submit.Page{}, submit.ErrForbidden
	}
	cursorTime, cursorSeq, err := decodeCursor(q.Cursor)
	if err != nil {
		return submit.Page{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT s.id::text, s.lab_id::text, s.state, s.received_at, s.server_seq,
		       s.judge_verdict, s.infrastructure_error, s.test_revision, s.constraint_revision
		FROM submissions s
		WHERE s.lab_id = $1
		  AND ($2 = '' OR s.author_user_id::text = $2)
		  AND ($3 = false OR (s.received_at, s.server_seq) < ($4, $5))
		ORDER BY s.received_at DESC, s.server_seq DESC
		LIMIT $6`, q.LabID, author, q.Cursor != "", cursorTime, cursorSeq, q.Limit+1)
	if err != nil {
		return submit.Page{}, fmt.Errorf("postgres: list submissions: %w", err)
	}
	defer rows.Close()
	type row struct {
		view       submit.View
		seq        int64
		verdict    *string
		infra      *string
		state      string
		testRev    int
		constraint int
	}
	var scanned []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.view.ID, &item.view.LabID, &item.state, &item.view.ReceivedAt, &item.seq, &item.verdict, &item.infra, &item.testRev, &item.constraint); err != nil {
			return submit.Page{}, fmt.Errorf("postgres: scan submission: %w", err)
		}
		scanned = append(scanned, item)
	}
	if err := rows.Err(); err != nil {
		return submit.Page{}, err
	}
	next := ""
	if len(scanned) > q.Limit {
		last := scanned[q.Limit-1]
		next = encodeCursor(last.view.ReceivedAt, last.seq)
		scanned = scanned[:q.Limit]
	}
	items := make([]submit.View, 0, len(scanned))
	for _, item := range scanned {
		visible, show := visibleState(item.state, item.verdict, item.infra)
		item.view.State = visible
		if show {
			tests, err := loadTests(ctx, s.pool, item.view.ID, item.view.LabID, item.testRev, item.constraint)
			if err != nil {
				return submit.Page{}, err
			}
			item.view.Judge = &submit.JudgeView{Verdict: *item.verdict, Tests: tests}
		}
		items = append(items, item.view)
	}
	return submit.Page{Items: items, NextCursor: next}, nil
}

// LeaseJudgeJobs claims pending or expired judge jobs and raises the attempt fence.
func (s *Store) LeaseJudgeJobs(ctx context.Context, workerID string, max int, now, expires time.Time) ([]domain.Job, error) {
	if workerID == "" || max < 1 || max > 16 || !expires.After(now) {
		return nil, errors.New("postgres: invalid lease")
	}
	var jobs []domain.Job
	err := s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		rows, err := tx.pg.Query(ctx, `
			SELECT id::text
			FROM jobs
			WHERE kind = 'judge'
			  AND (state = 'pending' OR (state = 'leased' AND lease_expires_at <= $1))
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2`, now, max)
		if err != nil {
			return fmt.Errorf("postgres: pick jobs: %w", err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			var job domain.Job
			var kind string
			err := tx.QueryRow(ctx, `
				UPDATE jobs
				SET state = 'leased',
				    attempt = attempt + 1,
				    lease_expires_at = $2,
				    worker_id = $3,
				    last_error = NULL
				WHERE id = $1
				RETURNING id::text, attempt, kind, submission_id::text, lab_id::text,
				          generation, test_revision, constraint_revision, source_sha256`,
				id, expires, workerID).Scan(
				&job.JobID, &job.Attempt, &kind, &job.SubmissionID, &job.LabID,
				&job.Generation, &job.TestRevision, &job.ConstraintRevision, &job.SourceSHA256)
			if err != nil {
				return fmt.Errorf("postgres: lease job: %w", err)
			}
			job.Version = 1
			job.Kind = domain.JobKind(kind)
			if err := tx.Exec(ctx, `
				UPDATE submissions
				SET state = 'judging'
				WHERE id = $1 AND state = 'queued'`, job.SubmissionID); err != nil {
				return err
			}
			jobs = append(jobs, job)
		}
		return nil
	})
	return jobs, err
}

// CommitVerdict stores one verdict set when the attempt fence still owns the job.
// A matching duplicate delivery does not insert another set. A stale attempt
// returns ErrStaleAttempt and writes nothing.
func (s *Store) CommitVerdict(ctx context.Context, job domain.Job, result judge.Result) error {
	if !judge.KnownVerdict(result.Verdict) || len(result.Tests) == 0 {
		return errors.New("postgres: verdict set is incomplete")
	}
	for _, test := range result.Tests {
		if test.ID == "" || !judge.KnownVerdict(test.Verdict) || test.TimeMS < 0 {
			return errors.New("postgres: verdict set is incomplete")
		}
	}
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		var state string
		var attempt int
		var submissionID, sha string
		err := tx.QueryRow(ctx, `
			SELECT state, attempt, submission_id::text, source_sha256
			FROM jobs
			WHERE id = $1
			FOR UPDATE`, job.JobID).Scan(&state, &attempt, &submissionID, &sha)
		if errors.Is(err, pgx.ErrNoRows) {
			return judge.ErrStaleAttempt
		}
		if err != nil {
			return fmt.Errorf("postgres: lock job: %w", err)
		}
		if attempt != job.Attempt || submissionID != job.SubmissionID || sha != job.SourceSHA256 {
			return judge.ErrStaleAttempt
		}
		if state == "done" {
			return nil
		}
		if state != "leased" {
			return judge.ErrStaleAttempt
		}
		tag, err := tx.pg.Exec(ctx, `
			INSERT INTO judge_verdicts (
				submission_id, test_revision, constraint_revision, attempt, aggregate_verdict
			) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (submission_id, test_revision, constraint_revision) DO NOTHING`,
			job.SubmissionID, job.TestRevision, job.ConstraintRevision, job.Attempt, result.Verdict)
		if err != nil {
			return fmt.Errorf("postgres: insert verdict: %w", err)
		}
		if tag.RowsAffected() == 1 {
			for _, test := range result.Tests {
				if err := tx.Exec(ctx, `
					INSERT INTO judge_result_tests (
						submission_id, test_revision, constraint_revision, test_id, verdict, time_ms
					) VALUES ($1, $2, $3, $4, $5, $6)`,
					job.SubmissionID, job.TestRevision, job.ConstraintRevision, test.ID, test.Verdict, test.TimeMS); err != nil {
					return fmt.Errorf("postgres: insert test verdict: %w", err)
				}
			}
			if err := tx.Exec(ctx, `
				UPDATE submissions
				SET state = 'judged', judge_verdict = $2, infrastructure_error = NULL
				WHERE id = $1`, job.SubmissionID, result.Verdict); err != nil {
				return err
			}
			if err := tx.Exec(ctx, `
				INSERT INTO jobs (
					id, submission_id, lab_id, kind, attempt, generation,
					test_revision, constraint_revision, source_sha256, state
				)
				SELECT gen_random_uuid(), $1, $2, 'check', 0, $3, $4, $5, $6, 'pending'
				WHERE $7 = 'AC'
				  AND EXISTS (
				      SELECT 1
				      FROM counted_selections c
				      JOIN submissions s ON s.id = $1
				      WHERE c.lab_id = $2
				        AND c.author_user_id = s.author_user_id
				        AND c.submission_id = $1
				        AND c.generation = $3
				  )
				ON CONFLICT (submission_id, kind, generation, test_revision, constraint_revision) DO NOTHING`,
				job.SubmissionID, job.LabID, job.Generation, job.TestRevision, job.ConstraintRevision, job.SourceSHA256, result.Verdict); err != nil {
				return fmt.Errorf("postgres: enqueue check: %w", err)
			}
		}
		if err := tx.Exec(ctx, `
			UPDATE jobs
			SET state = 'done', lease_expires_at = NULL
			WHERE id = $1 AND attempt = $2 AND state = 'leased'`, job.JobID, job.Attempt); err != nil {
			return err
		}
		return nil
	})
}

// FailJudge records an infrastructure failure for the current fence. It does
// not write a student verdict. A stale fence cannot record the failure either.
func (s *Store) FailJudge(ctx context.Context, job domain.Job, reason string) error {
	reason = cleanReason(reason)
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		var state string
		var attempt int
		err := tx.QueryRow(ctx, `
			SELECT state, attempt FROM jobs WHERE id = $1 FOR UPDATE`, job.JobID).Scan(&state, &attempt)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (attempt != job.Attempt || state != "leased")) {
			return judge.ErrStaleAttempt
		}
		if err != nil {
			return fmt.Errorf("postgres: lock failed job: %w", err)
		}
		next := "pending"
		if job.Attempt >= judge.MaxAttempts {
			next = "failed"
		}
		if err := tx.Exec(ctx, `
			UPDATE jobs
			SET state = $2, lease_expires_at = NULL, worker_id = NULL, last_error = $3
			WHERE id = $1 AND attempt = $4`, job.JobID, next, reason, job.Attempt); err != nil {
			return err
		}
		if next == "failed" {
			if err := tx.Exec(ctx, `
				UPDATE submissions
				SET infrastructure_error = $2
				WHERE id = $1 AND judge_verdict IS NULL`, job.SubmissionID, reason); err != nil {
				return err
			}
		}
		return nil
	})
}

// LeaseCheckJobs claims pending or expired check jobs. It does not change
// the student-visible submission state.
func (s *Store) LeaseCheckJobs(ctx context.Context, workerID string, max int, now, expires time.Time) ([]domain.Job, error) {
	if workerID == "" || max < 1 || max > 16 || !expires.After(now) {
		return nil, errors.New("postgres: invalid lease")
	}
	var jobs []domain.Job
	err := s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		rows, err := tx.pg.Query(ctx, `
			SELECT id::text
			FROM jobs
			WHERE kind = 'check'
			  AND (state = 'pending' OR (state = 'leased' AND lease_expires_at <= $1))
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2`, now, max)
		if err != nil {
			return fmt.Errorf("postgres: pick check jobs: %w", err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			var job domain.Job
			var kind string
			err := tx.QueryRow(ctx, `
				UPDATE jobs
				SET state = 'leased',
				    attempt = attempt + 1,
				    lease_expires_at = $2,
				    worker_id = $3,
				    last_error = NULL
				WHERE id = $1
				RETURNING id::text, attempt, kind, submission_id::text, lab_id::text,
				          generation, test_revision, constraint_revision, source_sha256`,
				id, expires, workerID).Scan(
				&job.JobID, &job.Attempt, &kind, &job.SubmissionID, &job.LabID,
				&job.Generation, &job.TestRevision, &job.ConstraintRevision, &job.SourceSHA256)
			if err != nil {
				return fmt.Errorf("postgres: lease check job: %w", err)
			}
			job.Version = 1
			job.Kind = domain.JobKind(kind)
			jobs = append(jobs, job)
		}
		return nil
	})
	return jobs, err
}

// CompleteCheck marks the leased check job done. A stale fence writes nothing.
func (s *Store) CompleteCheck(ctx context.Context, job domain.Job) error {
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		tag, err := tx.pg.Exec(ctx, `
			UPDATE jobs
			SET state = 'done', lease_expires_at = NULL
			WHERE id = $1 AND attempt = $2 AND state = 'leased' AND kind = 'check'`,
			job.JobID, job.Attempt)
		if err != nil {
			return fmt.Errorf("postgres: complete check: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return checking.ErrStale
		}
		return nil
	})
}

// FailCheck returns a leased check job to the queue, or marks it failed
// after three attempts. It does not write a student verdict.
func (s *Store) FailCheck(ctx context.Context, job domain.Job, reason string) error {
	reason = cleanReason(reason)
	return s.Within(ctx, func(ctx context.Context, tx *Tx) error {
		next := "pending"
		if job.Attempt >= 3 {
			next = "failed"
		}
		tag, err := tx.pg.Exec(ctx, `
			UPDATE jobs
			SET state = $2, lease_expires_at = NULL, worker_id = NULL, last_error = $3
			WHERE id = $1 AND attempt = $4 AND state = 'leased' AND kind = 'check'`,
			job.JobID, next, reason, job.Attempt)
		if err != nil {
			return fmt.Errorf("postgres: fail check: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return checking.ErrStale
		}
		return nil
	})
}

// LoadJudgeWork reads the immutable source and private tests for one job.
func (s *Store) LoadJudgeWork(ctx context.Context, job domain.Job) (judge.Work, error) {
	var work judge.Work
	var source string
	err := s.pool.QueryRow(ctx, `
		SELECT s.source, s.source_sha256, s.author_user_id::text, l.cpu_seconds, l.memory_mib
		FROM submissions s
		JOIN labs l ON l.id = s.lab_id
		WHERE s.id = $1 AND s.lab_id = $2`, job.SubmissionID, job.LabID).Scan(
		&source, &work.SourceSHA256, &work.AuthorUserID, &work.CPUSeconds, &work.MemoryMiB)
	if err != nil {
		return judge.Work{}, fmt.Errorf("postgres: load submission: %w", err)
	}
	work.Source = []byte(source)
	rows, err := s.pool.Query(ctx, `
		SELECT test_id, input, expected_output
		FROM lab_tests
		WHERE lab_id = $1 AND revision = $2
		ORDER BY position, test_id`, job.LabID, job.TestRevision)
	if err != nil {
		return judge.Work{}, fmt.Errorf("postgres: load tests: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tc judge.Case
		if err := rows.Scan(&tc.ID, &tc.Input, &tc.Expected); err != nil {
			return judge.Work{}, err
		}
		work.Tests = append(work.Tests, tc)
	}
	if err := rows.Err(); err != nil {
		return judge.Work{}, err
	}
	return work, nil
}

type rowQuery interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func readAccess(ctx context.Context, q rowQuery, userID, labID string, lockLab bool) (labAccess, error) {
	var access labAccess
	query := `
		SELECT class_id::text, opens_at, deadline, test_revision, constraint_revision, cpu_seconds, memory_mib
		FROM labs
		WHERE id = $1`
	if lockLab {
		query += ` FOR UPDATE`
	}
	err := q.QueryRow(ctx, query, labID).Scan(
		&access.classID, &access.opensAt, &access.deadline, &access.testRevision, &access.constraintRevision, &access.cpuSeconds, &access.memoryMiB)
	if errors.Is(err, pgx.ErrNoRows) {
		return labAccess{}, nil
	}
	if err != nil {
		return labAccess{}, fmt.Errorf("postgres: read lab: %w", err)
	}
	access.found = true
	access.role, access.enrolled, err = membership(ctx, q, userID, access.classID)
	if err != nil {
		return labAccess{}, err
	}
	return access, nil
}

func membership(ctx context.Context, q rowQuery, userID, classID string) (string, bool, error) {
	var role string
	err := q.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("postgres: read user: %w", err)
	}
	var exists int
	err = q.QueryRow(ctx, `
		SELECT 1 FROM memberships
		WHERE user_id = $1 AND class_id = $2 AND removed_at IS NULL`, userID, classID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return role, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("postgres: read membership: %w", err)
	}
	return role, true, nil
}

func visibleState(state string, verdict, infra *string) (string, bool) {
	hasVerdict := verdict != nil && *verdict != "" && (infra == nil || *infra == "")
	switch state {
	case "queued":
		return "queued", false
	case "judged":
		if hasVerdict {
			return "judged", true
		}
		return "judging", false
	default:
		return "judging", false
	}
}

func loadTests(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, submissionID, labID string, testRevision, constraintRevision int) ([]submit.TestView, error) {
	rows, err := q.Query(ctx, `
		SELECT jrt.test_id, jrt.verdict, jrt.time_ms
		FROM judge_result_tests jrt
		JOIN lab_tests lt
		  ON lt.lab_id = $2 AND lt.revision = $3 AND lt.test_id = jrt.test_id
		WHERE jrt.submission_id = $1
		  AND jrt.test_revision = $3
		  AND jrt.constraint_revision = $4
		ORDER BY lt.position, jrt.test_id`, submissionID, labID, testRevision, constraintRevision)
	if err != nil {
		return nil, fmt.Errorf("postgres: load verdict tests: %w", err)
	}
	defer rows.Close()
	var tests []submit.TestView
	for rows.Next() {
		var test submit.TestView
		if err := rows.Scan(&test.ID, &test.Verdict, &test.TimeMS); err != nil {
			return nil, err
		}
		tests = append(tests, test)
	}
	if tests == nil {
		tests = []submit.TestView{}
	}
	return tests, rows.Err()
}

func encodeCursor(received time.Time, seq int64) string {
	payload := strconv.FormatInt(received.UTC().UnixNano(), 10) + ":" + strconv.FormatInt(seq, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeCursor(raw string) (time.Time, int64, error) {
	if raw == "" {
		return time.Time{}, 0, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, 0, submit.ErrInvalid
	}
	nanoRaw, seqRaw, ok := strings.Cut(string(buf), ":")
	if !ok {
		return time.Time{}, 0, submit.ErrInvalid
	}
	nanos, err := strconv.ParseInt(nanoRaw, 10, 64)
	if err != nil {
		return time.Time{}, 0, submit.ErrInvalid
	}
	seq, err := strconv.ParseInt(seqRaw, 10, 64)
	if err != nil || seq < 1 {
		return time.Time{}, 0, submit.ErrInvalid
	}
	return time.Unix(0, nanos).UTC(), seq, nil
}

func cleanReason(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return "infrastructure failure"
	}
	if len(reason) > 240 {
		return reason[:240]
	}
	return reason
}
