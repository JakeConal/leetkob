package submit

import (
	"context"
	"errors"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
)

// Service applies the submission window to a repository. It does not write
// the HTTP response.
type Service struct {
	Repo  Repository
	Clock domain.Clock
}

// Begin opens an admission lease for an enrolled student. The body is not
// read yet.
func (s *Service) Begin(ctx context.Context, userID, labID string) (Admission, error) {
	if s == nil || s.Repo == nil {
		return Admission{}, errors.New("submit: store is not configured")
	}
	now := s.now()
	return s.Repo.BeginAdmission(ctx, userID, labID, now, now.Add(AdmissionLease))
}

// Complete stamps nothing itself: receivedAt is the instant the complete
// body arrived. A successful AcceptOut has already been committed.
func (s *Service) Complete(ctx context.Context, in AcceptIn, body []byte) (AcceptOut, error) {
	if s == nil || s.Repo == nil {
		return AcceptOut{}, errors.New("submit: store is not configured")
	}
	source, err := ParseSource(body)
	if err != nil {
		_ = s.Repo.AbortAdmission(ctx, in.LeaseID, in.ReceivedAt)
		return AcceptOut{}, ErrInvalid
	}
	in.Source = source
	out, err := s.Repo.Accept(ctx, in)
	if errors.Is(err, ErrReplayRace) {
		out, err = s.Repo.Accept(ctx, in)
	}
	return out, err
}

func (s *Service) now() time.Time {
	if s != nil && s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now().UTC()
}
