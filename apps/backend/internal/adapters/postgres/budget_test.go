package postgres

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

func TestBudgetEnforcesTheQuarterDollarCap(t *testing.T) {
	store, ctx := openSchema(t)
	budget := &Budget{Store: store}

	lab := insertLab(t, ctx, store)
	held, err := budget.Reserve(ctx, lab, semantic.LabCapMicroUSD, 1<<62)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := budget.Reserve(ctx, lab, 1, semantic.LabCapMicroUSD); !errors.Is(err, semantic.ErrBudgetExceeded) {
		t.Fatalf("over USD 0.25 err = %v", err)
	}
	if err := budget.Release(ctx, held.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE labs SET budget_cap_micros = 50 WHERE id = $1`, lab); err != nil {
		t.Fatal(err)
	}
	if _, err := budget.Reserve(ctx, lab, 51, semantic.LabCapMicroUSD); !errors.Is(err, semantic.ErrBudgetExceeded) {
		t.Fatalf("lab cap err = %v", err)
	}
	if _, err := budget.Reserve(ctx, lab, 50, semantic.LabCapMicroUSD); err != nil {
		t.Fatal(err)
	}

	caller := insertLab(t, ctx, store)
	if _, err := budget.Reserve(ctx, caller, 10, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := budget.Reserve(ctx, caller, 1, 10); !errors.Is(err, semantic.ErrBudgetExceeded) {
		t.Fatalf("caller cap err = %v", err)
	}
	charged, err := budget.Charged(ctx, caller)
	if err != nil || charged != 10 {
		t.Fatalf("charged = %d err %v", charged, err)
	}
}

func TestBudgetSettleKeepAndRelease(t *testing.T) {
	store, ctx := openSchema(t)
	budget := &Budget{Store: store}
	lab := insertLab(t, ctx, store)

	held, err := budget.Reserve(ctx, lab, 100, semantic.LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Settle(ctx, held.ID, 40); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, ctx, budget, lab, 40)
	if err := budget.Settle(ctx, held.ID, 0); err == nil {
		t.Fatal("second settle was accepted")
	}

	kept, err := budget.Reserve(ctx, lab, 100, semantic.LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Keep(ctx, kept.ID); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, ctx, budget, lab, 140)

	released, err := budget.Reserve(ctx, lab, 15, semantic.LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Release(ctx, released.ID); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, ctx, budget, lab, 140)
	if err := budget.Settle(ctx, "99999999-9999-4999-8999-999999999999", 1); !errors.Is(err, semantic.ErrReservationNotFound) {
		t.Fatalf("missing reservation err = %v", err)
	}
}

func TestConcurrentReservationsStayInsideTheCap(t *testing.T) {
	store, ctx := openSchema(t)
	budget := &Budget{Store: store}
	lab := insertLab(t, ctx, store)
	const (
		n      = 40
		amount = int64(1)
		cap    = int64(10)
	)
	var wg sync.WaitGroup
	var okCount atomic.Int32
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := budget.Reserve(ctx, lab, amount, cap)
			if err == nil {
				okCount.Add(1)
				return
			}
			if !errors.Is(err, semantic.ErrBudgetExceeded) {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if got := okCount.Load(); got != 10 {
		t.Fatalf("admitted = %d, want 10", got)
	}
	assertCharged(t, ctx, budget, lab, cap)
	if charged, _ := budget.Charged(ctx, lab); charged > semantic.LabCapMicroUSD {
		t.Fatalf("charged %d exceeds USD 0.25", charged)
	}
}

func TestFreeModelCallLimitsPersistAcrossBudgetInstances(t *testing.T) {
	store, ctx := openSchema(t)
	lab := insertLab(t, ctx, store)
	budget := &Budget{Store: store, MaxLabCalls: 2, MaxDailyCalls: 10}
	for i := 0; i < 2; i++ {
		held, err := budget.Reserve(ctx, lab, 0, semantic.LabCapMicroUSD)
		if err != nil {
			t.Fatal(err)
		}
		if err := budget.Settle(ctx, held.ID, 0); err != nil {
			t.Fatal(err)
		}
	}
	// A new worker/budget instance cannot reset the free-tier request quota.
	restarted := &Budget{Store: store, MaxLabCalls: 2, MaxDailyCalls: 10}
	if _, err := restarted.Reserve(ctx, lab, 0, semantic.LabCapMicroUSD); !errors.Is(err, semantic.ErrBudgetExceeded) {
		t.Fatalf("free call cap after restart: %v", err)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM budget_reservations WHERE lab_id=$1`, lab).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rejected free call changed stored reservations: count=%d err=%v", count, err)
	}
}

func TestConcurrentLabsCannotOversubscribeDailyFreeCalls(t *testing.T) {
	store, ctx := openSchema(t)
	labs := []string{insertLab(t, ctx, store), insertLab(t, ctx, store)}
	budget := &Budget{Store: store, MaxLabCalls: 20, MaxDailyCalls: 1}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var admitted atomic.Int32
	errorsSeen := make(chan error, 2)
	for _, lab := range labs {
		wg.Add(1)
		go func(lab string) {
			defer wg.Done()
			<-start
			_, err := budget.Reserve(ctx, lab, 0, semantic.LabCapMicroUSD)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, semantic.ErrBudgetExceeded) {
				errorsSeen <- err
			}
		}(lab)
	}
	close(start)
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if admitted.Load() != 1 {
		t.Fatalf("daily quota admitted %d calls, want exactly one", admitted.Load())
	}
}

func TestSemanticCheckerReservesInPostgresWithoutNetwork(t *testing.T) {
	store, ctx := openSchema(t)
	budget := &Budget{Store: store}
	lab := insertLab(t, ctx, store)
	price := semantic.Price{Known: true, MaxInputTokens: 100, MaxOutputTokens: 20, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 1}
	fake := &pgModel{body: `{"results":[{"constraintId":"c1","verdict":"pass","evidence":[]}]}`, in: 10, out: 5}
	checker := &semantic.Checker{Model: fake, Store: budget, Timeout: time.Second}
	input := semantic.Input{
		LabID: lab, Statement: "Count.", Constraints: []semantic.Constraint{{ID: "c1", Text: "Be brief."}},
		Source: "int main(){return 0;}\n", Price: price,
	}
	result, err := checker.Check(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 || result.Findings[0].Verdict != semantic.VerdictPass {
		t.Fatalf("calls %d finding %+v", fake.calls.Load(), result.Findings)
	}
	assertCharged(t, ctx, budget, lab, 15)

	remainder, err := budget.Reserve(ctx, lab, semantic.LabCapMicroUSD-15, semantic.LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Keep(ctx, remainder.ID); err != nil {
		t.Fatal(err)
	}
	second := &pgModel{body: fake.body, in: 10, out: 5}
	blocked, err := (&semantic.Checker{Model: second, Store: budget, Timeout: time.Second}).Check(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if second.calls.Load() != 0 || blocked.Findings[0].UnsureCause != semantic.CauseBudget {
		t.Fatalf("calls %d finding %+v", second.calls.Load(), blocked.Findings)
	}
	assertCharged(t, ctx, budget, lab, semantic.LabCapMicroUSD)
}

func openSchema(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	store, _ := openDisposable(t, ctx)
	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return store, ctx
}

func insertLab(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()
	userID := newUUID(t)
	classID := newUUID(t)
	labID := newUUID(t)
	err := store.Within(ctx, func(ctx context.Context, tx *Tx) error {
		if err := tx.Exec(ctx, `
			INSERT INTO users (id, email, display_name, role, password_hash)
			VALUES ($1, $2, 'Budget TA', 'ta', 'not-a-login')`, userID, userID+"@example.test"); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `INSERT INTO classes (id, name) VALUES ($1, 'Budget')`, classID); err != nil {
			return err
		}
		return tx.Exec(ctx, `
			INSERT INTO labs (id, class_id, created_by, title, statement, opens_at, deadline, cpu_seconds, memory_mib)
			VALUES ($1, $2, $3, 'Budget', 'Count the spend.', $4, $5, 1, 64)`,
			labID, classID, userID, time.Now().UTC(), time.Now().UTC().Add(time.Hour))
	})
	if err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	return labID
}

func assertCharged(t *testing.T, ctx context.Context, budget *Budget, lab string, want int64) {
	t.Helper()
	got, err := budget.Charged(ctx, lab)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("charged = %d, want %d", got, want)
	}
}

type pgModel struct {
	body  string
	in    int64
	out   int64
	calls atomic.Int32
}

func (m *pgModel) Complete(context.Context, semantic.CompletionRequest) (semantic.ModelResponse, error) {
	m.calls.Add(1)
	return semantic.ModelResponse{Content: m.body, TokensKnown: true, InputTokens: m.in, OutputTokens: m.out, RequestID: "fake-1"}, nil
}
