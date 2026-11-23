// Package domain holds the pure domain rules and injected ports for the
// PA#3 backend. Storage, transport and provider adapters depend on these
// interfaces; the domain never depends on them.
package domain

import (
	"context"
	"time"
)

// Clock supplies server time. Tests inject a fixed clock so deadline,
// lease and audit behavior is deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock returns real UTC server time.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// JobKind enumerates durable queue job kinds. It mirrors
// docs/contract/schemas/job-envelope.v1.json.
type JobKind string

const (
	// KindJudge compiles and executes one submission and stores exactly
	// one verdict set.
	KindJudge JobKind = "judge"
	// KindCheck runs hard rules and guarded semantic checks for current
	// counted Accepted work.
	KindCheck JobKind = "check"
)

// Job is the persisted durable queue envelope. Results echo the revision
// tuple and attempt fence; a stale fence or superseded revision tuple
// cannot commit. Field names mirror job-envelope.v1.json.
type Job struct {
	Version            int     `json:"version"`
	JobID              string  `json:"jobId"`
	Attempt            int     `json:"attempt"`
	Kind               JobKind `json:"kind"`
	SubmissionID       string  `json:"submissionId"`
	LabID              string  `json:"labId"`
	Generation         int     `json:"generation"`
	TestRevision       int     `json:"testRevision"`
	ConstraintRevision int     `json:"constraintRevision"`
	SourceSHA256       string  `json:"sourceSha256"`
}

// Queue is the durable job source leased by a separate worker process.
// The concrete adapter lands with the PostgreSQL store (issue #38 task
// 2.1/4.2); the port keeps the worker independent of storage.
type Queue interface {
	// Lease claims at most max pending jobs for this worker. Each claimed
	// job carries its current attempt fence.
	Lease(ctx context.Context, max int) ([]Job, error)
}
