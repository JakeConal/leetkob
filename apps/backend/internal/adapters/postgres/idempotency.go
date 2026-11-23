package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrIdempotencyNotFound is returned when no completed mutation response is
// stored for the actor/operation/resource/key tuple.
var ErrIdempotencyNotFound = errors.New("postgres: idempotency record not found")

// IdempotencyRecord is one completed mutation response keyed by actor,
// operation, resource and caller-supplied idempotency key.
type IdempotencyRecord struct {
	ActorID        string
	Operation      string
	ResourceID     string
	IdempotencyKey string
	Fingerprint    string
	Status         int
	ContentType    string
	Body           []byte
}

// FindIdempotency reads one completed response.
func (s *Store) FindIdempotency(ctx context.Context, actorID, operation, resourceID, key string) (IdempotencyRecord, error) {
	var record IdempotencyRecord
	if err := s.pool.QueryRow(ctx, `
		SELECT actor_id, operation, resource_id, idempotency_key, fingerprint,
			status, content_type, body
		FROM idempotency_keys
		WHERE actor_id = $1 AND operation = $2 AND resource_id = $3 AND idempotency_key = $4`,
		actorID, operation, resourceID, key,
	).Scan(&record.ActorID, &record.Operation, &record.ResourceID, &record.IdempotencyKey,
		&record.Fingerprint, &record.Status, &record.ContentType, &record.Body,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IdempotencyRecord{}, ErrIdempotencyNotFound
		}
		return IdempotencyRecord{}, fmt.Errorf("postgres: find idempotency record: %w", err)
	}
	return record, nil
}

// SaveIdempotency stores one completed response. The primary key makes a
// concurrent duplicate an explicit conflict rather than an overwrite.
func (s *Store) SaveIdempotency(ctx context.Context, record IdempotencyRecord) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys
			(actor_id, operation, resource_id, idempotency_key, fingerprint, status, content_type, body)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		record.ActorID, record.Operation, record.ResourceID, record.IdempotencyKey,
		record.Fingerprint, record.Status, record.ContentType, string(record.Body),
	); err != nil {
		return fmt.Errorf("postgres: save idempotency record: %w", err)
	}
	return nil
}
