package compatpub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestPublicationBenchmark drives the same publication workload as the
// Node comparison script (scripts/compare-backends.mjs) against this Go
// compatibility composition and emits one BENCHJSON line with the
// measured metrics. It is skipped in short mode. Configuration comes from
// COMPARE_WARMUP, COMPARE_ROUNDS, COMPARE_CONCURRENCY and COMPARE_LABS
// with the same defaults the script passes to the Node side.
func TestPublicationBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark comparison is skipped in short mode")
	}

	warmup := envInt("COMPARE_WARMUP", 200)
	rounds := envInt("COMPARE_ROUNDS", 5)
	concurrency := envInt("COMPARE_CONCURRENCY", 50)
	labCount := envInt("COMPARE_LABS", 200)

	instant := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	secret := "comparison-secret-that-is-at-least-32-bytes"
	ta := Identity{Subject: "ta-1", Role: RoleTA, ClassIDs: []string{"class-a"}}
	student := Identity{Subject: "student-1", Role: RoleStudent, ClassIDs: []string{"class-a"}}

	seeded := make([]Lab, 0, labCount+1)
	for i := 0; i < labCount; i++ {
		seeded = append(seeded, readyBenchLab(labUUID(i), "class-a"))
	}
	// One lab owned by another class exercises the concealed 404 path.
	foreignID := labUUID(labCount)
	seeded = append(seeded, readyBenchLab(foreignID, "class-b"))

	labRepo := NewInMemoryLabRepository(seeded)
	started := time.Now()
	mux := NewMux(HandlerDeps{
		Secret:      secret,
		NowSeconds:  func() int64 { return 1_800_000_000 },
		Labs:        labRepo,
		Idempotency: NewInMemoryIdempotencyRepository(),
		Clock:       FixedClock{Instant: instant},
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	startupMs := float64(time.Since(started).Microseconds()) / 1000.0

	taToken, err := IssueToken(ta, secret, 1_800_000_060)
	if err != nil {
		t.Fatalf("issue ta token: %v", err)
	}
	studentToken, err := IssueToken(student, secret, 1_800_000_060)
	if err != nil {
		t.Fatalf("issue student token: %v", err)
	}

	post := func(token string, path string, key string) (int, time.Duration) {
		request, requestErr := http.NewRequest(http.MethodPost, server.URL+"/labs/"+path, nil)
		if requestErr != nil {
			t.Fatalf("build request: %v", requestErr)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Idempotency-Key", key)
		requestStart := time.Now()
		response, doErr := http.DefaultClient.Do(request)
		if doErr != nil {
			t.Fatalf("do request: %v", doErr)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return response.StatusCode, time.Since(requestStart)
	}

	// Warmup: replays on an already-published lab.
	for i := 0; i < warmup; i++ {
		if status, _ := post(taToken, labUUID(0)+"/publish", "warmup-key"); status != 200 {
			t.Fatalf("warmup request returned %d", status)
		}
	}

	var mu sync.Mutex
	latencies := make([]float64, 0, rounds*(labCount*4+concurrency*2))
	errors := 0
	record := func(status int, want int, elapsed time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		if status != want {
			errors++
			t.Errorf("unexpected status %d, want %d", status, want)
		}
		latencies = append(latencies, float64(elapsed.Microseconds())/1000.0)
	}

	start := time.Now()
	for round := 0; round < rounds; round++ {
		// Reset the round's labs to ready so every round measures the same
		// publication path against fresh state.
		for i := 1; i < labCount; i++ {
			if err := labRepo.Save(readyBenchLab(labUUID(i), "class-a")); err != nil {
				t.Fatalf("reset lab: %v", err)
			}
		}
		// Sequential phase: publication, replay, concealed 404, forbidden.
		for i := 1; i < labCount; i++ {
			labID := labUUID(i)
			key := fmt.Sprintf("publish-key-%d-%d", round, i)
			status, elapsed := post(taToken, labID+"/publish", key)
			record(status, 200, elapsed)
			status, elapsed = post(taToken, labID+"/publish", key)
			record(status, 200, elapsed)
			status, elapsed = post(taToken, foreignID+"/publish", "missing-"+key)
			record(status, 404, elapsed)
			status, elapsed = post(studentToken, labID+"/publish", "student-"+key)
			record(status, 403, elapsed)
		}
		// Concurrent phase: different keys race on one fresh lab.
		fresh := labUUID(labCount + 1 + round)
		if err := labRepo.Save(readyBenchLab(fresh, "class-a")); err != nil {
			t.Fatalf("save fresh lab: %v", err)
		}
		var wg sync.WaitGroup
		for i := 0; i < concurrency*2; i++ {
			key := fmt.Sprintf("concurrent-%d-%d", round, i)
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				status, elapsed := post(taToken, fresh+"/publish", key)
				if status != 200 && status != 409 {
					mu.Lock()
					errors++
					mu.Unlock()
					return
				}
				record(status, status, elapsed)
			}(key)
		}
		wg.Wait()
	}
	elapsedTotal := time.Since(start)

	sortFloats(latencies)
	total := len(latencies)
	metrics := map[string]any{
		"implementation": "go-compatpub",
		"requests":       total + warmup,
		"errors":         errors,
		"warmup":         warmup,
		"rounds":         rounds,
		"concurrency":    concurrency,
		"labs":           labCount,
		"latencyMs": map[string]float64{
			"mean": mean(latencies),
			"p50":  percentile(latencies, 50),
			"p95":  percentile(latencies, 95),
			"max":  maxFloat(latencies),
		},
		"throughputPerSecond": float64(total) / elapsedTotal.Seconds(),
		"measuredMs":          float64(elapsedTotal.Microseconds()) / 1000.0,
		"startupMs":           startupMs,
	}
	encoded, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}
	fmt.Printf("BENCHJSON:%s\n", encoded)
}

func readyBenchLab(id string, classID string) Lab {
	return Lab{
		ID:                 id,
		ClassID:            classID,
		Status:             StatusReady,
		PendingReviews:     0,
		SubmissionDeadline: "2026-11-22T22:00:00.000Z",
	}
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func labUUID(i int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := len(sorted) * p / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func sortFloats(values []float64) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
