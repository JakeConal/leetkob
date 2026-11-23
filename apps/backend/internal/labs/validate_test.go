package labs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateAcceptsTwoConstraintsAndHiddenTests(t *testing.T) {
	draft, err := Validate(mustJSON(t, baseRequest()))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if draft.CPUSeconds != 1 || draft.MemoryMiB != 256 {
		t.Fatalf("limits = %d %d", draft.CPUSeconds, draft.MemoryMiB)
	}
	if len(draft.Constraints) != 2 || draft.Constraints[0].Deduction != 300 || draft.Constraints[1].Deduction != 400 {
		t.Fatalf("deductions = %#v", draft.Constraints)
	}
	if len(draft.Tests) != 2 || !draft.Tests[0].Sample || draft.Tests[1].Sample || draft.Tests[1].Input != "hidden-sentinel\n" {
		t.Fatalf("tests = %#v", draft.Tests)
	}
	if !draft.OpensAt.Before(draft.Deadline) {
		t.Fatal("schedule must be strictly increasing")
	}
}

func TestValidateRejectsTheAuthoringFaults(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		raw  string
	}{
		{name: "equal schedule", edit: func(body map[string]any) {
			body["deadline"] = body["opensAt"]
		}},
		{name: "reversed schedule", edit: func(body map[string]any) {
			body["opensAt"] = "2026-11-26T00:00:00Z"
		}},
		{name: "duplicate constraint ids", edit: func(body map[string]any) {
			body["constraints"] = []map[string]any{
				{"id": "C1", "text": "one", "deduction": 1},
				{"id": "C1", "text": "two", "deduction": 2},
			}
		}},
		{name: "cpu above range", edit: func(body map[string]any) {
			body["limits"] = map[string]any{"cpuSeconds": 6, "memoryMiB": 256}
		}},
		{name: "cpu fraction", edit: func(body map[string]any) {
			body["limits"] = map[string]any{"cpuSeconds": 1.5, "memoryMiB": 256}
		}},
		{name: "memory below range", edit: func(body map[string]any) {
			body["limits"] = map[string]any{"cpuSeconds": 1, "memoryMiB": 63}
		}},
		{name: "compiler command", edit: func(body map[string]any) {
			body["compiler"] = "g++ -std=c++17 -O2"
		}},
		{name: "filesystem path", edit: func(body map[string]any) {
			body["path"] = "/tmp/student.cpp"
		}},
		{name: "deduction too fine", edit: func(body map[string]any) {
			constraints := body["constraints"].([]map[string]any)
			constraints[0]["deduction"] = 1.234
		}},
		{name: "no constraints", edit: func(body map[string]any) {
			body["constraints"] = []map[string]any{}
		}},
		{name: "no tests", edit: func(body map[string]any) {
			body["tests"] = []map[string]any{}
		}},
		{name: "trailing json", raw: string(mustJSON(t, baseRequest())) + `{"extra":true}`},
		{name: "duplicate key", raw: `{"title":"A","title":"B","statement":"s","opensAt":"2026-11-20T04:00:00Z","deadline":"2026-11-25T15:00:00Z","constraints":[{"id":"C1","text":"t","deduction":1}],"tests":[{"id":"01","input":"a","expectedOutput":"b","sample":true}],"limits":{"cpuSeconds":1,"memoryMiB":64}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.raw)
			if tc.raw == "" {
				body := baseRequest()
				tc.edit(body)
				raw = mustJSON(t, body)
			}
			if _, err := Validate(raw); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want invalid", err)
			}
		})
	}
}

func TestValidateBoundaryLimits(t *testing.T) {
	low := baseRequest()
	low["limits"] = map[string]any{"cpuSeconds": 1, "memoryMiB": 64}
	if _, err := Validate(mustJSON(t, low)); err != nil {
		t.Fatalf("minimum limits: %v", err)
	}
	high := baseRequest()
	high["limits"] = map[string]any{"cpuSeconds": 5, "memoryMiB": 512}
	high["constraints"].([]map[string]any)[0]["deduction"] = 10
	high["constraints"].([]map[string]any)[1]["deduction"] = 0
	if _, err := Validate(mustJSON(t, high)); err != nil {
		t.Fatalf("maximum limits: %v", err)
	}
	over := baseRequest()
	over["statement"] = strings.Repeat("a", MaxStatementBytes+1)
	if _, err := Validate(mustJSON(t, over)); !errors.Is(err, ErrInvalid) {
		t.Fatal("statement above 64 KiB must be rejected")
	}
}

func TestPresentDetailDropsHiddenTests(t *testing.T) {
	view := PresentDetail(VisibleLab{
		ID: "6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa", ClassID: "6f0c1a10-7b21-4a11-8c11-bbbbbbbbbbbb",
		Title: "sort-merge", Statement: "statement", Revision: 1,
		CPUSeconds: 2, MemoryMiB: 256,
		Constraints: []Constraint{{ID: "C1", Text: "Do not call `std::sort`.", Deduction: 300}},
		SampleTests: []Test{
			{ID: "02", Input: "sample", ExpectedOutput: "out", Sample: true},
			{ID: "05", Input: "hidden-sentinel", ExpectedOutput: "secret", Sample: false},
		},
	})
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "sample") || strings.Contains(string(encoded), "hidden-sentinel") {
		t.Fatalf("projection leaked or dropped the sample: %s", encoded)
	}
	if view.ModelNotice != ModelNotice || !strings.Contains(ModelNotice, "sanitized") || !strings.Contains(ModelNotice, "approves") {
		t.Fatal("model notice must explain sanitization and human approval")
	}
}

func baseRequest() map[string]any {
	return map[string]any{
		"title":     "Sorting lab",
		"statement": "Implement merge sort.",
		"opensAt":   "2026-11-20T04:00:00Z",
		"deadline":  "2026-11-25T15:00:00Z",
		"constraints": []map[string]any{
			{"id": "C1", "text": "Do not call `std::sort`.", "deduction": 3},
			{"id": "C2", "text": "Implement merge sort.", "deduction": 4},
		},
		"tests": []map[string]any{
			{"id": "01", "input": "1\n", "expectedOutput": "1\n", "sample": true},
			{"id": "02", "input": "hidden-sentinel\n", "expectedOutput": "nope\n", "sample": false},
		},
		"limits": map[string]any{"cpuSeconds": 1, "memoryMiB": 256},
	}
}

func mustJSON(t *testing.T, body map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
