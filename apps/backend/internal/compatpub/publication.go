package compatpub

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ProblemBase is the PA#2 problem namespace.
const ProblemBase = "https://leetkob.example/problems"

// LabStatus mirrors the PA#2 lab states.
type LabStatus string

const (
	StatusReady     LabStatus = "ready"
	StatusPublished LabStatus = "published"
)

// Lab is the seeded or published lab record. The optional publication
// fields carry meaning only when Status is published.
type Lab struct {
	ID                 string    `json:"id"`
	ClassID            string    `json:"classId"`
	Status             LabStatus `json:"status"`
	PendingReviews     int       `json:"pendingReviews"`
	SubmissionDeadline string    `json:"submissionDeadline"`
	PublishedAt        string    `json:"publishedAt,omitempty"`
	PublishedBy        string    `json:"publishedBy,omitempty"`
	PublicationKey     string    `json:"publicationKey,omitempty"`
}

// StoredResponse is the exact response a caller received, stored for
// exact-response idempotency.
type StoredResponse struct {
	Status      int
	ContentType string
	Body        map[string]any
}

// LabRepository stores labs. Save failures surface as errors so the
// handler can return the fixed generic problem without mutating state.
type LabRepository interface {
	FindByID(id string) (Lab, bool)
	Save(lab Lab) error
}

// IdempotencyRepository stores the first completed response per scope.
type IdempotencyRepository interface {
	Find(scope string) (StoredResponse, bool)
	SaveIfAbsent(scope string, response StoredResponse)
}

// Clock supplies server time; scenarios inject a fixed clock.
type Clock interface {
	Now() time.Time
}

// PublishInput carries one verified request into the domain.
type PublishInput struct {
	Identity       Identity
	LabID          string
	IdempotencyKey string
	Instance       string
}

// PublicationService applies the PA#2 publication rules in the exact
// documented order and stores the first completed result per
// (subject, labId, idempotency key) scope. Work for one lab is serialized
// so concurrent different keys yield exactly one publication.
type PublicationService struct {
	labs    LabRepository
	idem    IdempotencyRepository
	clock   Clock
	mu      sync.Mutex
	labLock map[string]*sync.Mutex
}

// NewPublicationService builds the service over injected adapters.
func NewPublicationService(labs LabRepository, idem IdempotencyRepository, clock Clock) *PublicationService {
	return &PublicationService{labs: labs, idem: idem, clock: clock, labLock: map[string]*sync.Mutex{}}
}

func (s *PublicationService) labMutex(labID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, ok := s.labLock[labID]
	if !ok {
		lock = &sync.Mutex{}
		s.labLock[labID] = lock
	}
	return lock
}

// Publish evaluates and stores one publication request. A repository
// error is returned to the caller and no response is stored.
func (s *PublicationService) Publish(input PublishInput) (StoredResponse, error) {
	lock := s.labMutex(input.LabID)
	lock.Lock()
	defer lock.Unlock()

	scope, err := idempotencyScope(input)
	if err != nil {
		return StoredResponse{}, err
	}
	if replay, ok := s.idem.Find(scope); ok {
		return replay, nil
	}
	response := s.evaluate(input)
	if response.Status == 0 {
		return StoredResponse{}, fmt.Errorf("compatpub: repository failure")
	}
	s.idem.SaveIfAbsent(scope, response)
	return response, nil
}

func (s *PublicationService) evaluate(input PublishInput) StoredResponse {
	if input.Identity.Role != RoleTA {
		return problemResponse("ta-role-required", 403, "TA role required",
			"Only a teaching assistant may publish a lab.", input.Instance)
	}

	lab, ok := s.labs.FindByID(input.LabID)
	if !ok || !contains(input.Identity.ClassIDs, lab.ClassID) {
		return problemResponse("lab-not-found", 404, "Lab not found",
			"No lab is available to this teaching assistant at the requested identifier.", input.Instance)
	}

	if lab.Status == StatusPublished {
		return problemResponse("lab-already-published", 409, "Lab already published",
			"The lab was already published using a different idempotency key.", input.Instance,
			"labId", lab.ID, "publishedAt", lab.PublishedAt, "publishedBy", lab.PublishedBy)
	}

	publicationTime := s.clock.Now()
	deadline, err := time.Parse(time.RFC3339, lab.SubmissionDeadline)
	if err != nil {
		return internalErrorResponse(input.Instance)
	}
	if publicationTime.Before(deadline) {
		return problemResponse("lab-still-open", 409, "Submissions still open",
			"Marks cannot be published before the submission deadline.", input.Instance,
			"submissionDeadline", lab.SubmissionDeadline)
	}

	if lab.PendingReviews > 0 {
		return problemResponse("reviews-pending", 409, "Reviews pending",
			"All required reviews must be resolved before marks can be published.", input.Instance,
			"pendingReviews", lab.PendingReviews)
	}

	publishedAt := formatInstant(publicationTime)
	published := lab
	published.Status = StatusPublished
	published.PendingReviews = 0
	published.PublishedAt = publishedAt
	published.PublishedBy = input.Identity.Subject
	published.PublicationKey = input.IdempotencyKey
	if err := s.labs.Save(published); err != nil {
		return StoredResponse{}
	}

	return StoredResponse{
		Status:      200,
		ContentType: "application/json",
		Body: map[string]any{
			"labId":       published.ID,
			"status":      "published",
			"publishedAt": publishedAt,
			"publishedBy": input.Identity.Subject,
		},
	}
}

// idempotencyScope reproduces the exact JSON array serialization used by
// the Node implementation: ["subject","labId","key"].
func idempotencyScope(input PublishInput) (string, error) {
	encoded, err := json.Marshal([]string{input.Identity.Subject, input.LabID, input.IdempotencyKey})
	if err != nil {
		return "", fmt.Errorf("compatpub: marshal scope: %w", err)
	}
	return string(encoded), nil
}

// problemResponse builds an RFC 9457 body with optional extensions.
func problemResponse(code string, status int, title, detail, instance string, extensions ...any) StoredResponse {
	body := map[string]any{
		"type":     ProblemBase + "/" + code,
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": instance,
	}
	for i := 0; i+1 < len(extensions); i += 2 {
		if key, ok := extensions[i].(string); ok {
			body[key] = extensions[i+1]
		}
	}
	return StoredResponse{Status: status, ContentType: "application/problem+json", Body: body}
}

// internalErrorResponse is the fixed generic 500 body; it never echoes
// error text, storage details or credentials.
func internalErrorResponse(instance string) StoredResponse {
	return problemResponse("internal-error", 500, "Internal server error",
		"The request could not be completed.", instance)
}

// formatInstant renders UTC time exactly like ECMAScript
// Date.prototype.toISOString with millisecond precision.
func formatInstant(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000") + "Z"
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// InMemoryLabRepository is the PA#2 in-memory adapter with copy-out reads.
type InMemoryLabRepository struct {
	mu   sync.RWMutex
	labs map[string]Lab
}

// NewInMemoryLabRepository seeds the repository with cloned labs.
func NewInMemoryLabRepository(labs []Lab) *InMemoryLabRepository {
	repository := &InMemoryLabRepository{labs: map[string]Lab{}}
	for _, lab := range labs {
		repository.labs[lab.ID] = lab
	}
	return repository
}

// FindByID returns a copy of the stored lab.
func (r *InMemoryLabRepository) FindByID(id string) (Lab, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lab, ok := r.labs[id]
	return lab, ok
}

// Save stores a copy of the lab.
func (r *InMemoryLabRepository) Save(lab Lab) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.labs[lab.ID] = lab
	return nil
}

// InMemoryIdempotencyRepository keeps the first completed response per
// scope.
type InMemoryIdempotencyRepository struct {
	mu        sync.Mutex
	responses map[string]StoredResponse
}

// NewInMemoryIdempotencyRepository builds the adapter.
func NewInMemoryIdempotencyRepository() *InMemoryIdempotencyRepository {
	return &InMemoryIdempotencyRepository{responses: map[string]StoredResponse{}}
}

// Find returns the stored response for a scope.
func (r *InMemoryIdempotencyRepository) Find(scope string) (StoredResponse, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	response, ok := r.responses[scope]
	return response, ok
}

// SaveIfAbsent stores the response only when the scope has none.
func (r *InMemoryIdempotencyRepository) SaveIfAbsent(scope string, response StoredResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.responses[scope]; !ok {
		r.responses[scope] = response
	}
}

// FixedClock always reports the same instant.
type FixedClock struct {
	Instant time.Time
}

// Now implements Clock.
func (c FixedClock) Now() time.Time { return c.Instant }
