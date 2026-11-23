package access

import "time"

const (
	// BucketLoginIP counts failed sign-ins for one client address.
	BucketLoginIP = "login-ip"
	// BucketLoginAccount counts failed sign-ins for one account identifier.
	BucketLoginAccount = "login-account"

	ipWindow         = time.Minute
	accountWindow    = 15 * time.Minute
	maxLoginFailures = 10
)

// WindowStart is the inclusive start of the fixed window that contains now.
func WindowStart(now time.Time, window time.Duration) time.Time {
	return now.UTC().Truncate(window)
}

// IPWindowStart is the one-minute bucket for a client address.
func IPWindowStart(now time.Time) time.Time { return WindowStart(now, ipWindow) }

// AccountWindowStart is the fifteen-minute bucket for an account identifier.
func AccountWindowStart(now time.Time) time.Time { return WindowStart(now, accountWindow) }

// RetryAfterSeconds is the whole seconds until the window ends, at least 1.
func RetryAfterSeconds(now, windowStart time.Time, window time.Duration) int {
	seconds := int(windowStart.Add(window).Sub(now.UTC()).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

// rateDecision blocks the attempt when either counter is already at the
// limit. The delay is the longer of the windows that are exhausted, so a
// caller cannot slip through the shorter one.
func rateDecision(ipCount, accountCount int, now, ipStart, accountStart time.Time) (bool, int) {
	retry := 0
	blocked := false
	if ipCount >= maxLoginFailures {
		blocked = true
		retry = RetryAfterSeconds(now, ipStart, ipWindow)
	}
	if accountCount >= maxLoginFailures {
		blocked = true
		if wait := RetryAfterSeconds(now, accountStart, accountWindow); wait > retry {
			retry = wait
		}
	}
	return blocked, retry
}
