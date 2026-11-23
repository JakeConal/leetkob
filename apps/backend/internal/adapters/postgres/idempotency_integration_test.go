package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestIdempotencyRecordsRoundTripAndConflict(t *testing.T) {
	pool := migratedPool(t)
	seedIdentity(t, pool)
	store := &Store{pool: pool}
	ctx := context.Background()

	record := IdempotencyRecord{
		ActorID:        taID,
		Operation:      "create-lab",
		ResourceID:     classID,
		IdempotencyKey: "round-trip-key",
		Fingerprint:    "fingerprint-one",
		Status:         201,
		ContentType:    "application/json; charset=utf-8",
		Body:           []byte(`{"id":"a2000000-0000-4000-8000-000000000001","revision":1}`),
	}
	if err := store.SaveIdempotency(ctx, record); err != nil {
		t.Fatalf("save idempotency record: %v", err)
	}

	read, err := store.FindIdempotency(ctx, taID, "create-lab", classID, "round-trip-key")
	if err != nil {
		t.Fatalf("find idempotency record: %v", err)
	}
	if read.ActorID != record.ActorID || read.Operation != record.Operation ||
		read.ResourceID != record.ResourceID || read.IdempotencyKey != record.IdempotencyKey ||
		read.Fingerprint != record.Fingerprint || read.Status != record.Status ||
		read.ContentType != record.ContentType {
		t.Fatalf("idempotency record mismatch: %+v", read)
	}
	if string(read.Body) != string(record.Body) {
		var got, want map[string]any
		if err := json.Unmarshal(read.Body, &got); err != nil {
			t.Fatalf("decode stored idempotency body: %v", err)
		}
		if err := json.Unmarshal(record.Body, &want); err != nil {
			t.Fatalf("decode expected idempotency body: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("idempotency body = %s, want %s", read.Body, record.Body)
		}
	}

	if _, err := store.FindIdempotency(ctx, taID, "create-lab", classID, "unknown-key"); !errors.Is(err, ErrIdempotencyNotFound) {
		t.Fatalf("unknown key error = %v, want ErrIdempotencyNotFound", err)
	}
	// The key is scoped to actor/operation/resource, not global.
	if _, err := store.FindIdempotency(ctx, studentID, "create-lab", classID, "round-trip-key"); !errors.Is(err, ErrIdempotencyNotFound) {
		t.Fatalf("other actor key error = %v, want ErrIdempotencyNotFound", err)
	}

	if err := store.SaveIdempotency(ctx, record); err == nil {
		t.Fatal("duplicate idempotency key must conflict, not overwrite")
	}
}
