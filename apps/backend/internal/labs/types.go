package labs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	// CompileSeconds, WallMultiplier, MaxProcesses, MaxOutputBytes and
	// MaxSourceBytes are the fixed execution policy. Callers cannot send
	// a compiler command or a filesystem path.
	CompileSeconds    = 10
	WallMultiplier    = 3
	MaxProcesses      = 16
	MaxOutputBytes    = 1 << 20
	MaxSourceBytes    = 65536
	BudgetCapMicros   = 250000
	MaxStatementBytes = 64 * 1024
	MaxTestPayload    = 10 * 1024 * 1024
	MaxConstraints    = 20
	MaxTests          = 500

	// ModelNotice tells a reader that model input is sanitized and that a
	// person approves any deduction before it is released.
	ModelNotice = "Model input is sanitized before review: comments and known student identities are removed, and the remaining source is untrusted data. A teaching assistant approves any deduction before it is released."

	// CreateOperation is the idempotency scope for lab creation.
	CreateOperation = "POST /v2/classes/{classId}/labs"
)

var (
	// ErrInvalid is a rejected lab body. The HTTP boundary maps it to the
	// generic invalid-request problem and does not echo the reason.
	ErrInvalid = errors.New("labs: invalid request")
	// ErrConflict is an idempotency key reused with different content.
	ErrConflict = errors.New("labs: conflict")
	// ErrDuplicateReplay is the unique-key race on a concurrent create.
	// The service reads the stored result instead of inserting again.
	ErrDuplicateReplay = errors.New("labs: duplicate replay")
	// ErrTooLarge is a body the server refused to finish reading.
	ErrTooLarge = errors.New("labs: payload too large")
)

// Hundredths is a deduction stored as an integer number of hundredths of
// a point. JSON uses at most two decimal places and never a binary float.
type Hundredths int

func (h Hundredths) MarshalJSON() ([]byte, error) {
	v := int(h)
	if v < 0 || v > 1000 {
		return nil, errors.New("labs: deduction out of range")
	}
	switch {
	case v%100 == 0:
		return []byte(strconv.Itoa(v / 100)), nil
	case v%10 == 0:
		return fmt.Appendf(nil, "%d.%d", v/100, (v%100)/10), nil
	default:
		return fmt.Appendf(nil, "%d.%02d", v/100, v%100), nil
	}
}

func (h *Hundredths) UnmarshalJSON(raw []byte) error {
	n, err := parseDeduction(json.Number(raw))
	if err != nil {
		return err
	}
	*h = Hundredths(n)
	return nil
}

// Constraint is one TA-authored rule and its deduction.
type Constraint struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Deduction Hundredths `json:"deduction"`
}

// Test is one input and expected output. Sample tests are visible to
// enrolled readers. Hidden tests are stored and omitted from responses.
type Test struct {
	ID             string `json:"id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	Sample         bool   `json:"sample"`
}

// LimitsView is the TA-chosen CPU and memory policy returned by the API.
// The fixed compile, wall, process, output and source limits are stored
// on the lab and are not extra response fields.
type LimitsView struct {
	CPUSeconds int `json:"cpuSeconds"`
	MemoryMiB  int `json:"memoryMiB"`
}

// Draft is a validated creation request, including hidden tests.
type Draft struct {
	Title       string
	Statement   string
	OpensAt     time.Time
	Deadline    time.Time
	Constraints []Constraint
	Tests       []Test
	CPUSeconds  int
	MemoryMiB   int
}

// StoredLab is the row set inserted in one transaction.
type StoredLab struct {
	ID                 string
	ClassID            string
	CreatedBy          string
	Title              string
	Statement          string
	OpensAt            time.Time
	Deadline           time.Time
	CPUSeconds         int
	MemoryMiB          int
	CompileSeconds     int
	WallMultiplier     int
	MaxProcesses       int
	MaxOutputBytes     int
	MaxSourceBytes     int
	BudgetCapMicros    int64
	Revision           int
	TestRevision       int
	ConstraintRevision int
	Generation         int
	Constraints        []Constraint
	Tests              []Test
}

// Replay is the stored success body for one idempotency key.
type Replay struct {
	ActorID     string
	Operation   string
	Resource    string
	Key         string
	Fingerprint string
	Status      int
	Body        []byte
}

// Summary is one row of a class lab list.
type Summary struct {
	ID        string
	ClassID   string
	Title     string
	OpensAt   time.Time
	Deadline  time.Time
	Revision  int
	CreatedAt time.Time
}

// VisibleLab is a lab the caller may read. SampleTests must not contain
// hidden bodies. PresentDetail filters again before encoding.
type VisibleLab struct {
	ID          string
	ClassID     string
	Title       string
	Statement   string
	OpensAt     time.Time
	Deadline    time.Time
	Revision    int
	Constraints []Constraint
	CPUSeconds  int
	MemoryMiB   int
	SampleTests []Test
}

// LabView is the 201 body. It has no tests.
type LabView struct {
	ID          string       `json:"id"`
	ClassID     string       `json:"classId"`
	Title       string       `json:"title"`
	Statement   string       `json:"statement"`
	OpensAt     string       `json:"opensAt"`
	Deadline    string       `json:"deadline"`
	Revision    int          `json:"revision"`
	Constraints []Constraint `json:"constraints"`
	Limits      LimitsView   `json:"limits"`
	ModelNotice string       `json:"modelNotice"`
}

// LabDetailView is the student and TA read body, plus sample tests only.
type LabDetailView struct {
	LabView
	SampleTests []Test `json:"sampleTests"`
}

// SummaryView is one list item.
type SummaryView struct {
	ID       string `json:"id"`
	ClassID  string `json:"classId"`
	Title    string `json:"title"`
	OpensAt  string `json:"opensAt"`
	Deadline string `json:"deadline"`
	Revision int    `json:"revision"`
}

// FormatInstant renders UTC with exactly three fractional digits.
func FormatInstant(t time.Time) string {
	return t.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
}

// PresentLab builds the creation response. Revision is always 1 here.
func PresentLab(lab StoredLab) LabView {
	return LabView{
		ID:          lab.ID,
		ClassID:     lab.ClassID,
		Title:       lab.Title,
		Statement:   lab.Statement,
		OpensAt:     FormatInstant(lab.OpensAt),
		Deadline:    FormatInstant(lab.Deadline),
		Revision:    lab.Revision,
		Constraints: append([]Constraint(nil), lab.Constraints...),
		Limits:      LimitsView{CPUSeconds: lab.CPUSeconds, MemoryMiB: lab.MemoryMiB},
		ModelNotice: ModelNotice,
	}
}

// PresentDetail copies the visible lab and drops any test that is not a
// sample, so a store mistake cannot put a hidden body on the wire.
func PresentDetail(lab VisibleLab) LabDetailView {
	samples := make([]Test, 0, len(lab.SampleTests))
	for _, test := range lab.SampleTests {
		if test.Sample {
			samples = append(samples, test)
		}
	}
	return LabDetailView{
		LabView: LabView{
			ID:          lab.ID,
			ClassID:     lab.ClassID,
			Title:       lab.Title,
			Statement:   lab.Statement,
			OpensAt:     FormatInstant(lab.OpensAt),
			Deadline:    FormatInstant(lab.Deadline),
			Revision:    lab.Revision,
			Constraints: append([]Constraint(nil), lab.Constraints...),
			Limits:      LimitsView{CPUSeconds: lab.CPUSeconds, MemoryMiB: lab.MemoryMiB},
			ModelNotice: ModelNotice,
		},
		SampleTests: samples,
	}
}

// PresentSummary formats one list row.
func PresentSummary(item Summary) SummaryView {
	return SummaryView{
		ID:       item.ID,
		ClassID:  item.ClassID,
		Title:    item.Title,
		OpensAt:  FormatInstant(item.OpensAt),
		Deadline: FormatInstant(item.Deadline),
		Revision: item.Revision,
	}
}
