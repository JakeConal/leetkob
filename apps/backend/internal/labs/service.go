package labs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/access"
)

// Repository stores labs and the creation replay record.
type Repository interface {
	CurrentMember(ctx context.Context, userID, classID string) (bool, error)
	FindReplay(ctx context.Context, actorID, operation, resource, key string) (Replay, bool, error)
	CreateLab(ctx context.Context, lab StoredLab, replay Replay) error
	ListLabs(ctx context.Context, userID, role, classID string, now time.Time, limit int, cursor string) ([]Summary, string, error)
	ReadLab(ctx context.Context, userID, role, labID string, now time.Time) (VisibleLab, bool, error)
}

// Create inserts a lab for a TA who currently belongs to classID.
// Validation failures and conflicts leave no lab row. The same idempotency
// key and the same body return the original lab. A different body for that
// key returns ErrConflict.
func Create(ctx context.Context, repo Repository, actor access.Principal, classID, idempotencyKey string, body []byte) (LabView, error) {
	if !actor.CanCreateLab() {
		return LabView{}, access.ErrForbidden
	}
	member, err := repo.CurrentMember(ctx, actor.User.ID, classID)
	if err != nil {
		return LabView{}, err
	}
	if !member {
		return LabView{}, access.ErrNotFound
	}
	draft, err := Validate(body)
	if err != nil {
		return LabView{}, err
	}
	fingerprint := hex.EncodeToString(sha256Sum(body))
	existing, found, err := repo.FindReplay(ctx, actor.User.ID, CreateOperation, classID, idempotencyKey)
	if err != nil {
		return LabView{}, err
	}
	if found {
		return replayView(existing, fingerprint)
	}
	id, err := newID()
	if err != nil {
		return LabView{}, err
	}
	stored := StoredLab{
		ID:                 id,
		ClassID:            classID,
		CreatedBy:          actor.User.ID,
		Title:              draft.Title,
		Statement:          draft.Statement,
		OpensAt:            draft.OpensAt,
		Deadline:           draft.Deadline,
		CPUSeconds:         draft.CPUSeconds,
		MemoryMiB:          draft.MemoryMiB,
		CompileSeconds:     CompileSeconds,
		WallMultiplier:     WallMultiplier,
		MaxProcesses:       MaxProcesses,
		MaxOutputBytes:     MaxOutputBytes,
		MaxSourceBytes:     MaxSourceBytes,
		BudgetCapMicros:    BudgetCapMicros,
		Revision:           1,
		TestRevision:       1,
		ConstraintRevision: 1,
		Generation:         1,
		Constraints:        draft.Constraints,
		Tests:              draft.Tests,
	}
	view := PresentLab(stored)
	encoded, err := json.Marshal(view)
	if err != nil {
		return LabView{}, err
	}
	replay := Replay{
		ActorID:     actor.User.ID,
		Operation:   CreateOperation,
		Resource:    classID,
		Key:         idempotencyKey,
		Fingerprint: fingerprint,
		Status:      201,
		Body:        encoded,
	}
	if err := repo.CreateLab(ctx, stored, replay); err != nil {
		if errors.Is(err, ErrDuplicateReplay) {
			again, ok, readErr := repo.FindReplay(ctx, actor.User.ID, CreateOperation, classID, idempotencyKey)
			if readErr != nil {
				return LabView{}, readErr
			}
			if ok {
				return replayView(again, fingerprint)
			}
		}
		return LabView{}, err
	}
	return view, nil
}

func replayView(existing Replay, fingerprint string) (LabView, error) {
	if existing.Fingerprint != fingerprint {
		return LabView{}, ErrConflict
	}
	var view LabView
	if err := json.Unmarshal(existing.Body, &view); err != nil {
		return LabView{}, err
	}
	return view, nil
}

func sha256Sum(body []byte) []byte {
	sum := sha256.Sum256(body)
	return sum[:]
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("labs: id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
