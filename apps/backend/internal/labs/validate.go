package labs

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	constraintIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
	uuidPattern         = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type requestBody struct {
	Title       string          `json:"title"`
	Statement   string          `json:"statement"`
	OpensAt     string          `json:"opensAt"`
	Deadline    string          `json:"deadline"`
	Constraints []constraintReq `json:"constraints"`
	Tests       []testReq       `json:"tests"`
	Limits      limitsReq       `json:"limits"`
}

type constraintReq struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Deduction json.Number `json:"deduction"`
}

type testReq struct {
	ID             string `json:"id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expectedOutput"`
	Sample         bool   `json:"sample"`
}

type limitsReq struct {
	CPU    json.RawMessage `json:"cpuSeconds"`
	Memory json.RawMessage `json:"memoryMiB"`
}

// Validate accepts one lab-creation JSON document. Unknown members,
// duplicate keys, trailing values, a schedule that is not strictly
// increasing, duplicate ids, and limits outside the published ranges
// fail as ErrInvalid. The returned draft still contains hidden tests;
// those are stored and not returned by the read projection.
func Validate(raw []byte) (Draft, error) {
	if err := checkJSONObject(raw); err != nil {
		return Draft{}, ErrInvalid
	}
	if err := requireExactKeys(raw); err != nil {
		return Draft{}, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var body requestBody
	if err := dec.Decode(&body); err != nil {
		return Draft{}, ErrInvalid
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Draft{}, ErrInvalid
	}
	return validateBody(body)
}

func validateBody(body requestBody) (Draft, error) {
	if chars := utf8.RuneCountInString(body.Title); chars < 1 || chars > 200 {
		return Draft{}, ErrInvalid
	}
	if n := len(body.Statement); n < 1 || n > MaxStatementBytes || utf8.RuneCountInString(body.Statement) > MaxStatementBytes {
		return Draft{}, ErrInvalid
	}
	opensAt, err := parseInstant(body.OpensAt)
	if err != nil {
		return Draft{}, ErrInvalid
	}
	deadline, err := parseInstant(body.Deadline)
	if err != nil || !opensAt.Before(deadline) {
		return Draft{}, ErrInvalid
	}
	if n := len(body.Constraints); n < 1 || n > MaxConstraints {
		return Draft{}, ErrInvalid
	}
	if n := len(body.Tests); n < 1 || n > MaxTests {
		return Draft{}, ErrInvalid
	}
	constraints := make([]Constraint, len(body.Constraints))
	seenConstraints := make(map[string]struct{}, len(body.Constraints))
	for i, item := range body.Constraints {
		if !constraintIDPattern.MatchString(item.ID) {
			return Draft{}, ErrInvalid
		}
		if _, dup := seenConstraints[item.ID]; dup {
			return Draft{}, ErrInvalid
		}
		seenConstraints[item.ID] = struct{}{}
		if chars := utf8.RuneCountInString(item.Text); chars < 1 || chars > 4096 {
			return Draft{}, ErrInvalid
		}
		deduction, err := parseDeduction(item.Deduction)
		if err != nil {
			return Draft{}, ErrInvalid
		}
		constraints[i] = Constraint{ID: item.ID, Text: item.Text, Deduction: Hundredths(deduction)}
	}
	tests := make([]Test, len(body.Tests))
	seenTests := make(map[string]struct{}, len(body.Tests))
	payload := 0
	for i, item := range body.Tests {
		if chars := utf8.RuneCountInString(item.ID); chars < 1 || chars > 64 {
			return Draft{}, ErrInvalid
		}
		if _, dup := seenTests[item.ID]; dup {
			return Draft{}, ErrInvalid
		}
		seenTests[item.ID] = struct{}{}
		payload += len(item.Input) + len(item.ExpectedOutput)
		if payload > MaxTestPayload {
			return Draft{}, ErrInvalid
		}
		tests[i] = Test{ID: item.ID, Input: item.Input, ExpectedOutput: item.ExpectedOutput, Sample: item.Sample}
	}
	cpu, err := parseStrictInt(body.Limits.CPU, 1, 5)
	if err != nil {
		return Draft{}, ErrInvalid
	}
	memory, err := parseStrictInt(body.Limits.Memory, 64, 512)
	if err != nil {
		return Draft{}, ErrInvalid
	}
	return Draft{
		Title:       body.Title,
		Statement:   body.Statement,
		OpensAt:     opensAt,
		Deadline:    deadline,
		Constraints: constraints,
		Tests:       tests,
		CPUSeconds:  cpu,
		MemoryMiB:   memory,
	}, nil
}

func parseInstant(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	if _, offset := parsed.Zone(); offset != 0 {
		return time.Time{}, ErrInvalid
	}
	return parsed.UTC(), nil
}

func parseDeduction(number json.Number) (int, error) {
	text := strings.TrimSpace(string(number))
	if text == "" || strings.ContainsAny(text, "eE+") || strings.HasPrefix(text, "-") {
		return 0, ErrInvalid
	}
	if strings.HasPrefix(text, "\"") {
		return 0, ErrInvalid
	}
	whole, frac := text, ""
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		whole, frac = text[:dot], text[dot+1:]
		if whole == "" || len(frac) == 0 || len(frac) > 2 {
			return 0, ErrInvalid
		}
	}
	if whole != "0" && strings.HasPrefix(whole, "0") {
		return 0, ErrInvalid
	}
	w, err := strconv.Atoi(whole)
	if err != nil {
		return 0, ErrInvalid
	}
	f := 0
	if frac != "" {
		if len(frac) == 1 {
			frac += "0"
		}
		f, err = strconv.Atoi(frac)
		if err != nil {
			return 0, ErrInvalid
		}
	}
	hundredths := w*100 + f
	if hundredths < 0 || hundredths > 1000 {
		return 0, ErrInvalid
	}
	return hundredths, nil
}

func parseStrictInt(raw json.RawMessage, min, max int) (int, error) {
	text := bytes.TrimSpace(raw)
	if len(text) == 0 || text[0] == '"' {
		return 0, ErrInvalid
	}
	if bytes.ContainsAny(text, ".eE+-") {
		return 0, ErrInvalid
	}
	n, err := strconv.Atoi(string(text))
	if err != nil || n < min || n > max {
		return 0, ErrInvalid
	}
	return n, nil
}

func checkJSONObject(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return ErrInvalid
	}
	if err := walkObjectBody(dec); err != nil {
		return ErrInvalid
	}
	if len(bytes.TrimSpace(raw[dec.InputOffset():])) != 0 {
		return ErrInvalid
	}
	return nil
}

func walkValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return walkObjectBody(dec)
	case '[':
		return walkArrayBody(dec)
	default:
		return ErrInvalid
	}
}

func walkObjectBody(dec *json.Decoder) error {
	seen := make(map[string]struct{})
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return ErrInvalid
		}
		if _, dup := seen[key]; dup {
			return ErrInvalid
		}
		seen[key] = struct{}{}
		if err := walkValue(dec); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		return ErrInvalid
	}
	return nil
}

func walkArrayBody(dec *json.Decoder) error {
	for dec.More() {
		if err := walkValue(dec); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim(']') {
		return ErrInvalid
	}
	return nil
}

func requireExactKeys(raw []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return ErrInvalid
	}
	if !sameKeys(top, "title", "statement", "opensAt", "deadline", "constraints", "tests", "limits") {
		return ErrInvalid
	}
	var limits map[string]json.RawMessage
	if err := json.Unmarshal(top["limits"], &limits); err != nil {
		return ErrInvalid
	}
	if !sameKeys(limits, "cpuSeconds", "memoryMiB") {
		return ErrInvalid
	}
	var constraints []map[string]json.RawMessage
	if err := json.Unmarshal(top["constraints"], &constraints); err != nil {
		return ErrInvalid
	}
	for _, item := range constraints {
		if !sameKeys(item, "id", "text", "deduction") {
			return ErrInvalid
		}
	}
	var tests []map[string]json.RawMessage
	if err := json.Unmarshal(top["tests"], &tests); err != nil {
		return ErrInvalid
	}
	for _, item := range tests {
		if !sameKeys(item, "id", "input", "expectedOutput", "sample") {
			return ErrInvalid
		}
	}
	return nil
}

func sameKeys(obj map[string]json.RawMessage, keys ...string) bool {
	if len(obj) != len(keys) {
		return false
	}
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	return true
}
