package semantic

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentReservationsCannotOverspend(t *testing.T) {
	store := NewMemoryStore()
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
			_, err := store.Reserve(context.Background(), "lab-1", amount, cap)
			if err == nil {
				okCount.Add(1)
				return
			}
			if !errors.Is(err, ErrBudgetExceeded) {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("Reserve: %v", err)
	}
	if got := okCount.Load(); got != 10 {
		t.Fatalf("admitted = %d, want 10", got)
	}
	assertCharged(t, store, "lab-1", cap)
	if _, err := store.Reserve(context.Background(), "lab-1", 1, cap); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("over-cap reserve err = %v", err)
	}
	// A different lab has its own cap.
	if _, err := store.Reserve(context.Background(), "lab-2", cap, cap); err != nil {
		t.Fatalf("other lab: %v", err)
	}
}

func TestSettleReleasesRemainderAndKeepRetainsIt(t *testing.T) {
	store := NewMemoryStore()
	held, err := store.Reserve(context.Background(), "lab-1", 100, LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Settle(context.Background(), held.ID, 40); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, store, "lab-1", 40)

	kept, err := store.Reserve(context.Background(), "lab-1", 100, LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Keep(context.Background(), kept.ID); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, store, "lab-1", 140)

	released, err := store.Reserve(context.Background(), "lab-1", 15, LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Release(context.Background(), released.ID); err != nil {
		t.Fatal(err)
	}
	assertCharged(t, store, "lab-1", 140)
}

func TestUnknownPriceRefusesDispatch(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	fake := &scriptedModel{responses: []ModelResponse{{Content: passJSON("c1"), TokensKnown: true, InputTokens: 1, OutputTokens: 1}}}
	in := sampleInput()
	in.Price.Known = false
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 0 {
		t.Fatalf("calls = %d, want 0", fake.calls())
	}
	assertUnsure(t, res, "c1", CausePricing)
	assertCharged(t, store, "lab-1", 0)

	in.Price = Price{Known: true, MaxInputTokens: 1 << 62, MaxOutputTokens: 4, InputMicroUSDPerToken: 4, OutputMicroUSDPerToken: 1}
	res, err = (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check overflow: %v", err)
	}
	assertUnsure(t, res, "c1", CausePricing)
	if fake.calls() != 0 {
		t.Fatal("unbounded price was dispatched")
	}
}

func TestUnknownUsageKeepsReservationAcrossRecheck(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	price := Price{Known: true, MaxInputTokens: 200_000, MaxOutputTokens: 0, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 0}
	fake := &scriptedModel{responses: []ModelResponse{{Content: passJSON("c1"), TokensKnown: false}}}
	in := sampleInput()
	in.Price = price
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Findings[0].Verdict != VerdictPass {
		t.Fatalf("finding = %+v", res.Findings[0])
	}
	assertCharged(t, store, "lab-1", 200_000)

	// A recheck is another reservation against the same lab cap.
	second := &scriptedModel{responses: []ModelResponse{{Content: passJSON("c1"), TokensKnown: true, InputTokens: 1, OutputTokens: 0}}}
	res, err = (&Checker{Model: second, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("recheck: %v", err)
	}
	if second.calls() != 0 {
		t.Fatalf("recheck calls = %d, want 0", second.calls())
	}
	assertUnsure(t, res, "c1", CauseBudget)
	assertCharged(t, store, "lab-1", 200_000)
	if charged, _ := store.Charged(context.Background(), "lab-1"); charged > LabCapMicroUSD {
		t.Fatalf("charged %d exceeds USD 0.25", charged)
	}
}

func TestRetryStopsWhenTheSecondReservationDoesNotFit(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	// Leave room for one 1_500 hold. Unknown usage keeps that hold, so the
	// schema-repair reservation cannot fit inside USD 0.25.
	pre, err := store.Reserve(context.Background(), "lab-1", LabCapMicroUSD-1_500, LabCapMicroUSD)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Keep(context.Background(), pre.ID); err != nil {
		t.Fatal(err)
	}
	price := Price{Known: true, MaxInputTokens: 1_500, MaxOutputTokens: 0, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 0}
	fake := &scriptedModel{responses: []ModelResponse{
		{Content: `{"score":1}`, TokensKnown: false},
		{Content: passJSON("c1"), TokensKnown: true, InputTokens: 10, OutputTokens: 0},
	}}
	in := sampleInput()
	in.Price = price
	res, err := (&Checker{Model: fake, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if fake.calls() != 1 {
		t.Fatalf("calls = %d, want 1 when the retry reservation does not fit", fake.calls())
	}
	assertUnsure(t, res, "c1", CauseSchema)
	assertCharged(t, store, "lab-1", LabCapMicroUSD)
}

func TestRecheckSpendsTheSameLabBudget(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	price := Price{Known: true, MaxInputTokens: 1_000, MaxOutputTokens: 0, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 0}
	first := &scriptedModel{responses: []ModelResponse{{Content: passJSON("c1"), TokensKnown: true, InputTokens: 10, OutputTokens: 0}}}
	in := sampleInput()
	in.Price = price
	if _, err := (&Checker{Model: first, Store: store, Timeout: time.Second}).Check(context.Background(), in); err != nil {
		t.Fatalf("first check: %v", err)
	}
	second := &scriptedModel{responses: []ModelResponse{{Content: passJSON("c1"), TokensKnown: true, InputTokens: 15, OutputTokens: 0}}}
	res, err := (&Checker{Model: second, Store: store, Timeout: time.Second}).Check(context.Background(), in)
	if err != nil {
		t.Fatalf("recheck: %v", err)
	}
	if first.calls() != 1 || second.calls() != 1 || res.Findings[0].Verdict != VerdictPass {
		t.Fatalf("first=%d second=%d finding=%+v", first.calls(), second.calls(), res.Findings)
	}
	assertCharged(t, store, "lab-1", 25)
}

func TestConcurrentChecksHoldTheCapUntilTheyFinish(t *testing.T) {
	blockNetwork(t)
	store := NewMemoryStore()
	price := Price{Known: true, MaxInputTokens: 200_000, MaxOutputTokens: 0, InputMicroUSDPerToken: 1, OutputMicroUSDPerToken: 0}
	release := make(chan struct{})
	var entered atomic.Int32
	fake := &blockingModel{release: release, entered: &entered, content: passJSON("c1")}

	const n = 8
	type outcome struct {
		res Result
		err error
	}
	results := make(chan outcome, n)
	for i := 0; i < n; i++ {
		go func() {
			in := sampleInput()
			in.Price = price
			res, err := (&Checker{Model: fake, Store: store, Timeout: 5 * time.Second}).Check(context.Background(), in)
			results <- outcome{res, err}
		}()
	}

	deadline := time.Now().Add(2 * time.Second)
	for entered.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Let the losers return while the winner still holds 200_000.
	time.Sleep(50 * time.Millisecond)
	if got := entered.Load(); got != 1 {
		close(release)
		t.Fatalf("dispatches = %d, want 1 while the reservation is held", got)
	}
	assertCharged(t, store, "lab-1", 200_000)

	var maxSeen atomic.Int64
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			v, err := store.Charged(context.Background(), "lab-1")
			if err != nil {
				continue
			}
			for {
				old := maxSeen.Load()
				if v <= old || maxSeen.CompareAndSwap(old, v) {
					break
				}
			}
		}
	}()

	close(release)
	var budget, pass int
	for i := 0; i < n; i++ {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("Check: %v", got.err)
			}
			switch got.res.Findings[0].UnsureCause {
			case CauseBudget:
				budget++
				if got.res.ModelCalls != 0 {
					t.Fatalf("budget miss still dispatched: %+v", got.res)
				}
			case "":
				pass++
				if got.res.Findings[0].Verdict != VerdictPass || got.res.ModelCalls != 1 {
					t.Fatalf("winner = %+v", got.res)
				}
			default:
				t.Fatalf("unexpected finding %+v", got.res.Findings[0])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("checks did not finish")
		}
	}
	close(stop)
	if pass != 1 || budget != n-1 {
		t.Fatalf("pass=%d budget=%d, want 1 and %d", pass, budget, n-1)
	}
	// Winner reports one input token, so the hold shrinks from 200_000 to 1.
	assertCharged(t, store, "lab-1", 1)
	if maxSeen.Load() > LabCapMicroUSD {
		t.Fatalf("observed charged %d above USD 0.25", maxSeen.Load())
	}
}

type blockingModel struct {
	release chan struct{}
	entered *atomic.Int32
	content string
}

func (m *blockingModel) Complete(ctx context.Context, req CompletionRequest) (ModelResponse, error) {
	m.entered.Add(1)
	select {
	case <-m.release:
		return ModelResponse{Content: m.content, TokensKnown: true, InputTokens: 1, OutputTokens: 0}, nil
	case <-ctx.Done():
		return ModelResponse{}, ctx.Err()
	}
}
