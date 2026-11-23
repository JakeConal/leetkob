package access

import (
	"testing"
	"time"
)

func TestRateDecisionBlocksAtTenAndUsesTheLongerWindow(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 30, 0, time.UTC)
	ipStart := IPWindowStart(now)
	accountStart := AccountWindowStart(now)
	if ipStart != time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("ip window = %s", ipStart)
	}
	if accountStart != time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("account window = %s", accountStart)
	}

	blocked, retry := rateDecision(9, 9, now, ipStart, accountStart)
	if blocked || retry != 0 {
		t.Fatalf("ninth failure must still be allowed, blocked=%v retry=%d", blocked, retry)
	}
	blocked, retry = rateDecision(10, 0, now, ipStart, accountStart)
	if !blocked || retry != 30 {
		t.Fatalf("ip limit retry = %d, want 30 (blocked %v)", retry, blocked)
	}
	blocked, retry = rateDecision(10, 10, now, ipStart, accountStart)
	if !blocked || retry != 14*60+30 {
		t.Fatalf("both limits must wait out the 15 minute window, retry=%d", retry)
	}
}
