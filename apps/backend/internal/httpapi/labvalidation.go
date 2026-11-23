package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

const (
	maxLabTitleRunes      = 200
	maxLabStatementRunes  = 65536
	maxConstraintText     = 4096
	maxConstraintCount    = 20
	maxLabTests           = 500
	maxLabTestIDRunes     = 64
	minLabCPUSeconds      = 1
	maxLabCPUSeconds      = 5
	minLabMemoryMiB       = 64
	maxLabMemoryMiB       = 512
	maxDeductionCents     = 1000
	decimalToleranceCents = 1e-6
)

var constraintIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

type labCreateRequest struct {
	Title       string              `json:"title"`
	Statement   string              `json:"statement"`
	OpensAt     time.Time           `json:"opensAt"`
	Deadline    time.Time           `json:"deadline"`
	Constraints []constraintRequest `json:"constraints"`
	Tests       []testRequest       `json:"tests"`
	Limits      *limitsRequest      `json:"limits"`
}

type constraintRequest struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Deduction json.Number `json:"deduction"`
}

type testRequest struct {
	ID             string `json:"id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	Sample         *bool  `json:"sample"`
}

type limitsRequest struct {
	CPUSeconds int `json:"cpuSeconds"`
	MemoryMiB  int `json:"memoryMiB"`
}

// parseLabCreate validates the bounded request and builds the immutable
// revision-1 lab record. It accepts no compiler command or filesystem path:
// unknown members are rejected before this function is reached.
func parseLabCreate(body []byte, classID, creatorID string) (postgres.LabRecord, bool) {
	if !utf8.Valid(body) {
		return postgres.LabRecord{}, false
	}
	var request labCreateRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return postgres.LabRecord{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return postgres.LabRecord{}, false
	}

	titleLength := utf8.RuneCountInString(request.Title)
	if titleLength < 1 || titleLength > maxLabTitleRunes {
		return postgres.LabRecord{}, false
	}
	statementLength := utf8.RuneCountInString(request.Statement)
	if statementLength < 1 || statementLength > maxLabStatementRunes || len(request.Statement) > maxLabStatementRunes {
		return postgres.LabRecord{}, false
	}

	opensAt := request.OpensAt
	if opensAt.IsZero() {
		return postgres.LabRecord{}, false
	}
	deadline := request.Deadline
	if deadline.IsZero() || !opensAt.Before(deadline) {
		return postgres.LabRecord{}, false
	}

	if len(request.Constraints) < 1 || len(request.Constraints) > maxConstraintCount {
		return postgres.LabRecord{}, false
	}
	constraints := make([]postgres.ConstraintRecord, 0, len(request.Constraints))
	seenConstraints := make(map[string]struct{}, len(request.Constraints))
	for _, constraint := range request.Constraints {
		if !constraintIDPattern.MatchString(constraint.ID) {
			return postgres.LabRecord{}, false
		}
		if _, duplicate := seenConstraints[constraint.ID]; duplicate {
			return postgres.LabRecord{}, false
		}
		seenConstraints[constraint.ID] = struct{}{}
		textLength := utf8.RuneCountInString(constraint.Text)
		if textLength < 1 || textLength > maxConstraintText {
			return postgres.LabRecord{}, false
		}
		cents, ok := parseDeductionCents(constraint.Deduction)
		if !ok {
			return postgres.LabRecord{}, false
		}
		constraints = append(constraints, postgres.ConstraintRecord{
			ID: constraint.ID, Text: constraint.Text, DeductionCents: cents,
		})
	}

	if len(request.Tests) < 1 || len(request.Tests) > maxLabTests {
		return postgres.LabRecord{}, false
	}
	tests := make([]postgres.TestRecord, 0, len(request.Tests))
	seenTests := make(map[string]struct{}, len(request.Tests))
	testPayloadBytes := 0
	for _, test := range request.Tests {
		idLength := utf8.RuneCountInString(test.ID)
		if idLength < 1 || idLength > maxLabTestIDRunes || test.Sample == nil {
			return postgres.LabRecord{}, false
		}
		if _, duplicate := seenTests[test.ID]; duplicate {
			return postgres.LabRecord{}, false
		}
		seenTests[test.ID] = struct{}{}
		testPayloadBytes += len(test.Input) + len(test.ExpectedOutput)
		if testPayloadBytes > maxLabTestPayload {
			return postgres.LabRecord{}, false
		}
		tests = append(tests, postgres.TestRecord{
			ID: test.ID, Input: test.Input, ExpectedOutput: test.ExpectedOutput, Sample: *test.Sample,
		})
	}

	if request.Limits == nil ||
		request.Limits.CPUSeconds < minLabCPUSeconds || request.Limits.CPUSeconds > maxLabCPUSeconds ||
		request.Limits.MemoryMiB < minLabMemoryMiB || request.Limits.MemoryMiB > maxLabMemoryMiB {
		return postgres.LabRecord{}, false
	}

	id, err := newUUID()
	if err != nil {
		return postgres.LabRecord{}, false
	}
	return postgres.LabRecord{
		ID:          id,
		ClassID:     classID,
		Title:       request.Title,
		Statement:   request.Statement,
		OpensAt:     opensAt,
		Deadline:    deadline,
		Revision:    1,
		CPUSeconds:  request.Limits.CPUSeconds,
		MemoryMiB:   request.Limits.MemoryMiB,
		ModelNotice: modelNotice,
		CreatedBy:   creatorID,
		Constraints: constraints,
		Tests:       tests,
	}, true
}

func parseDeductionCents(number json.Number) (int, bool) {
	if number == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(string(number), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value < 0 || value > 10 {
		return 0, false
	}
	scaled := value * 100
	cents := int(math.Round(scaled))
	if math.Abs(scaled-float64(cents)) > decimalToleranceCents {
		return 0, false
	}
	if cents < 0 || cents > maxDeductionCents {
		return 0, false
	}
	return cents, true
}
