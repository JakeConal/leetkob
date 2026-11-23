package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Submission acceptance errors. They are distinct so the HTTP boundary can
// map them to the fixture-pinned problems.
var (
	// ErrLabNotAccepting means the lab is absent or the caller is not an
	// enrolled student of its class; the caller conceals both as 404.
	ErrLabNotAccepting = errors.New("postgres: lab not available for submission")
	// ErrSubmissionWindowClosed means the bounded request completed
	// outside opensAt <= receivedAt < deadline.
	ErrSubmissionWindowClosed = errors.New("postgres: submission window closed")
	// ErrIdempotencyContentConflict means a completed key was retried
	// with different content.
	ErrIdempotencyContentConflict = errors.New("postgres: idempotency content mismatch")
)

// SubmissionWindowClosedError carries the immutable deadline for the
// documented conflict extension.
type SubmissionWindowClosedError struct {
	Deadline time.Time
}

// Error implements error.
func (e *SubmissionWindowClosedError) Error() string { return "postgres: submission window closed" }

// AcceptCommand is one bounded submission request after authentication.
type AcceptCommand struct {
	ActorID        string
	LabID          string
	Source         string
	IdempotencyKey string
	Fingerprint    string
	ReceivedAt     time.Time
}

// AcceptResult is the durable outcome; Replayed marks a same-key replay.
type AcceptResult struct {
	SubmissionID string
	LabID        string
	State        string
	ReceivedAt   time.Time
	Replayed     bool
}

const submissionOperation = "create-submission"

// AcceptSubmission commits source, submission, counted selection, durable
// job intent and the replay record in one transaction. A same-key replay
// with identical content returns the stored result even after the
// deadline; different content conflicts. At or after the deadline a new
// request is refused and nothing is written.
func (s *Store) AcceptSubmission(ctx context.Context, command AcceptCommand) (AcceptResult, error) {
	digest := sha256.Sum256([]byte(command.Source))
	var result AcceptResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		replayed, found, err := findReplay(ctx, tx, command.ActorID, submissionOperation, command.LabID, command.IdempotencyKey, command.Fingerprint)
		if err != nil {
			return err
		}
		if found {
			result = replayed
			return nil
		}

		var classID string
		var opensAt, deadline time.Time
		var revision int
		if err := tx.QueryRow(ctx, `
			SELECT class_id, opens_at, deadline, revision FROM labs WHERE id = $1`,
			command.LabID,
		).Scan(&classID, &opensAt, &deadline, &revision); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrLabNotAccepting
			}
			return err
		}

		var enrolled bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM memberships WHERE class_id = $1 AND user_id = $2)`,
			classID, command.ActorID,
		).Scan(&enrolled); err != nil {
			return err
		}
		if !enrolled {
			return ErrLabNotAccepting
		}
		if command.ReceivedAt.Before(opensAt) || !command.ReceivedAt.Before(deadline) {
			return &SubmissionWindowClosedError{Deadline: deadline}
		}

		var generation int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(generation), 0) + 1 FROM submissions
			WHERE lab_id = $1 AND student_id = $2`, command.LabID, command.ActorID,
		).Scan(&generation); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE submissions SET counted = false
			WHERE lab_id = $1 AND student_id = $2 AND counted`, command.LabID, command.ActorID,
		); err != nil {
			return err
		}

		result = AcceptResult{LabID: command.LabID, State: "queued", ReceivedAt: command.ReceivedAt}
		if err := tx.QueryRow(ctx, `
			INSERT INTO submissions (id, lab_id, student_id, source, source_sha256, state,
				received_at, receipt_seq, generation, counted)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, 'queued', $5,
				nextval('submission_receipt_seq'), $6, true)
			RETURNING id`,
			command.LabID, command.ActorID, command.Source, digest[:], command.ReceivedAt, generation,
		).Scan(&result.SubmissionID); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO jobs (id, kind, submission_id, lab_id, generation, test_revision,
				constraint_revision, source_sha256, available_at)
			VALUES (gen_random_uuid(), 'judge', $1, $2, $3, $4, $4, $5, $6)`,
			result.SubmissionID, command.LabID, generation, revision, digest[:], command.ReceivedAt,
		); err != nil {
			return err
		}

		body, err := json.Marshal(map[string]any{
			"id": result.SubmissionID, "labId": result.LabID,
			"state": result.State, "receivedAt": result.ReceivedAt.UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO idempotency_keys (actor_id, operation, resource_id, idempotency_key,
				fingerprint, status, content_type, body)
			VALUES ($1, $2, $3, $4, $5, 202, 'application/json', $6)`,
			command.ActorID, submissionOperation, command.LabID, command.IdempotencyKey,
			command.Fingerprint, body,
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return AcceptResult{}, err
	}
	return result, nil
}

// findReplay returns a stored 202 submission result for the exact scope.
// A different fingerprint is a conflict, never a replay.
func findReplay(ctx context.Context, tx pgx.Tx, actorID, operation, resourceID, key, fingerprint string) (AcceptResult, bool, error) {
	var storedFingerprint string
	var body []byte
	if err := tx.QueryRow(ctx, `
		SELECT fingerprint, body FROM idempotency_keys
		WHERE actor_id = $1 AND operation = $2 AND resource_id = $3 AND idempotency_key = $4
		FOR UPDATE`, actorID, operation, resourceID, key,
	).Scan(&storedFingerprint, &body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AcceptResult{}, false, nil
		}
		return AcceptResult{}, false, fmt.Errorf("postgres: replay lookup: %w", err)
	}
	if storedFingerprint != fingerprint {
		return AcceptResult{}, false, ErrIdempotencyContentConflict
	}
	var stored struct {
		ID         string `json:"id"`
		LabID      string `json:"labId"`
		State      string `json:"state"`
		ReceivedAt string `json:"receivedAt"`
	}
	if err := json.Unmarshal(body, &stored); err != nil {
		return AcceptResult{}, false, fmt.Errorf("postgres: malformed stored replay: %w", err)
	}
	receivedAt, err := time.Parse(time.RFC3339Nano, stored.ReceivedAt)
	if err != nil {
		return AcceptResult{}, false, fmt.Errorf("postgres: malformed stored replay time: %w", err)
	}
	return AcceptResult{SubmissionID: stored.ID, LabID: stored.LabID, State: stored.State,
		ReceivedAt: receivedAt, Replayed: true}, true, nil
}
