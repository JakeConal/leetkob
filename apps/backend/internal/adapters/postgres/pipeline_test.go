package postgres

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/checking"
	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/judge"
	"github.com/JakeConal/leetkob/backend/internal/review"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

type acceptAll struct{}

func (acceptAll) Execute(context.Context, domain.Job) (judge.Result, error) {
	return judge.Result{Verdict: judge.VerdictAC, Tests: []judge.TestResult{{
		ID: "hidden", Verdict: judge.VerdictAC, TimeMS: 4,
	}}}, nil
}

type scriptedModel struct {
	calls int
	body  string
	seen  string
}

func (m *scriptedModel) Complete(_ context.Context, req semantic.CompletionRequest) (semantic.ModelResponse, error) {
	m.calls++
	m.seen = req.UserJSON
	return semantic.ModelResponse{Content: m.body, TokensKnown: true, InputTokens: 8, OutputTokens: 4, RequestID: "pipeline-fake"}, nil
}

func TestAcceptedSubmissionReachesTheReviewQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	store, _ := openDisposable(t, ctx)
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	opens := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	deadline := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	world := seedLab(t, ctx, store, opens, deadline)
	if err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		if err := tx.Exec(ctx, `
			INSERT INTO lab_constraints (lab_id, revision, constraint_id, position, body, deduction_hundredths)
			VALUES ($1, 1, 'ban-sort', 1, $2, 300)`, world.labID, "Do not use `std::sort`"); err != nil {
			return err
		}
		return tx.Exec(ctx, `
			INSERT INTO lab_constraints (lab_id, revision, constraint_id, position, body, deduction_hundredths)
			VALUES ($1, 1, 'readable', 2, 'The program is a complete program.', 100)`, world.labID)
	}); err != nil {
		t.Fatalf("constraints: %v", err)
	}

	source := "int main() {\n  std::sort(a, a + n);\n}\n"
	clock := &manualClock{t: deadline.Add(-time.Millisecond)}
	handler := newHandler(store, clock, world)
	accepted := postSubmission(t, handler, "student", world.labID, "submit-pipeline-1", `{"source":`+jsonString(source)+`}`)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", accepted.Code, accepted.Body.String())
	}
	submissionID := decodeAccept(t, accepted.Body.Bytes()).ID

	judgeWorker := &judge.Worker{
		Queue: store,
		Exec:  acceptAll{},
		ID:    "pipeline-judge",
		Now:   func() time.Time { return deadline },
	}
	n, err := judgeWorker.Process(ctx)
	if err != nil || n != 1 {
		t.Fatalf("judge process n=%d err=%v", n, err)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM jobs WHERE submission_id = $1 AND kind = 'check' AND state = 'pending'`, submissionID); got != 1 {
		t.Fatalf("pending check jobs = %d", got)
	}

	model := &scriptedModel{body: `{"results":[{"constraintId":"readable","verdict":"pass","evidence":[]}]}`}
	runner := &checking.Runner{
		Queue: store,
		ID:    "pipeline-check",
		Now:   func() time.Time { return deadline },
		Service: &checking.Service{
			Store: &FindingRepo{Store: store},
			Analyze: func(context.Context, string) ([]byte, error) {
				return []byte(`{"version":1,"status":"ok","references":[{"kind":"call","name":"sort","qualifiedName":"std::sort","header":"algorithm","resolved":true,"userDeclared":false,"line":2,"column":3}]}`), nil
			},
			Checker: &semantic.Checker{
				Model:   model,
				Store:   &Budget{Store: store},
				Timeout: time.Second,
			},
			Price: semantic.Price{Known: true, MaxInputTokens: 100, MaxOutputTokens: 50, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 1},
		},
	}
	n, err = runner.Process(ctx)
	if err != nil || n != 1 {
		t.Fatalf("check process n=%d err=%v", n, err)
	}
	if model.calls != 1 {
		t.Fatalf("model calls = %d", model.calls)
	}
	if strings.Contains(model.seen, "Do not use") || strings.Contains(model.seen, "ban-sort") {
		t.Fatalf("model saw the ban constraint: %s", model.seen)
	}
	if got := countRows(t, ctx, store, `SELECT count(*) FROM jobs WHERE submission_id = $1 AND kind = 'check' AND state = 'done'`, submissionID); got != 1 {
		t.Fatalf("done check jobs = %d", got)
	}

	page, err := (&ReviewRepo{Store: store}).List(ctx, world.labID, 10, "")
	if err != nil {
		t.Fatalf("review list: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("queue items = %d", len(page.Items))
	}
	got := map[string]review.Finding{}
	for _, item := range page.Items {
		got[item.ConstraintID] = item
	}
	ban := got["ban-sort"]
	if ban.CheckerVerdict != "violation" || ban.SourceKind != "hard-rule" || len(ban.Evidence) == 0 || !strings.Contains(ban.Evidence[0].Text, "std::sort") {
		t.Fatalf("ban finding = %+v", ban)
	}
	if got["readable"].CheckerVerdict != "pass" || got["readable"].SourceKind != "semantic" {
		t.Fatalf("semantic finding = %+v", got["readable"])
	}

	student := getSubmission(t, handler, "student", submissionID)
	if student.Code != http.StatusOK {
		t.Fatalf("student status = %d %s", student.Code, student.Body.String())
	}
	body := student.Body.String()
	if strings.Contains(body, "hard-rule") || strings.Contains(body, "checkerVerdict") || strings.Contains(body, hiddenExpected) {
		t.Fatalf("student body exposed checker or hidden output: %s", body)
	}
	if !strings.Contains(body, `"verdict":"AC"`) {
		t.Fatalf("student body = %s", body)
	}
}
