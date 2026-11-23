package httpapi

import (
	"encoding/json"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
)

type constraintView struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	Deduction float64 `json:"deduction"`
}

type limitsView struct {
	CPUSeconds int `json:"cpuSeconds"`
	MemoryMiB  int `json:"memoryMiB"`
}

type testItemView struct {
	ID             string `json:"id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	Sample         bool   `json:"sample"`
}

type labView struct {
	ID          string           `json:"id"`
	ClassID     string           `json:"classId"`
	Title       string           `json:"title"`
	Statement   string           `json:"statement"`
	OpensAt     string           `json:"opensAt"`
	Deadline    string           `json:"deadline"`
	Revision    int              `json:"revision"`
	Constraints []constraintView `json:"constraints"`
	Limits      limitsView       `json:"limits"`
	ModelNotice string           `json:"modelNotice"`
}

type labSummaryView struct {
	ID       string `json:"id"`
	ClassID  string `json:"classId"`
	Title    string `json:"title"`
	OpensAt  string `json:"opensAt"`
	Deadline string `json:"deadline"`
	Revision int    `json:"revision"`
}

type labListView struct {
	Items      []labSummaryView `json:"items"`
	NextCursor *string          `json:"nextCursor"`
}

type labDetailView struct {
	labView
	SampleTests []testItemView `json:"sampleTests"`
}

func viewConstraints(constraints []postgres.ConstraintRecord) []constraintView {
	views := make([]constraintView, 0, len(constraints))
	for _, constraint := range constraints {
		views = append(views, constraintView{
			ID:        constraint.ID,
			Text:      constraint.Text,
			Deduction: float64(constraint.DeductionCents) / 100,
		})
	}
	return views
}

func labRecordView(lab postgres.LabRecord) labView {
	return labView{
		ID:          lab.ID,
		ClassID:     lab.ClassID,
		Title:       lab.Title,
		Statement:   lab.Statement,
		OpensAt:     formatTime(lab.OpensAt),
		Deadline:    formatTime(lab.Deadline),
		Revision:    lab.Revision,
		Constraints: viewConstraints(lab.Constraints),
		Limits:      limitsView{CPUSeconds: lab.CPUSeconds, MemoryMiB: lab.MemoryMiB},
		ModelNotice: lab.ModelNotice,
	}
}

func labSummary(lab postgres.LabRecord) labSummaryView {
	return labSummaryView{
		ID:       lab.ID,
		ClassID:  lab.ClassID,
		Title:    lab.Title,
		OpensAt:  formatTime(lab.OpensAt),
		Deadline: formatTime(lab.Deadline),
		Revision: lab.Revision,
	}
}

// labDetail projects a lab and only its sample tests; hidden test inputs and
// expected outputs are never serialized.
func labDetail(lab postgres.LabRecord) labDetailView {
	samples := make([]testItemView, 0, len(lab.Tests))
	for _, test := range lab.Tests {
		if !test.Sample {
			continue
		}
		samples = append(samples, testItemView{
			ID: test.ID, Input: test.Input, ExpectedOutput: test.ExpectedOutput, Sample: true,
		})
	}
	return labDetailView{labView: labRecordView(lab), SampleTests: samples}
}

func marshalLab(lab postgres.LabRecord) []byte {
	payload, err := json.Marshal(labRecordView(lab))
	if err != nil {
		return nil
	}
	return payload
}
