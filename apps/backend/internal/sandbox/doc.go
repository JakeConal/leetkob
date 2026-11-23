// Package sandbox is the trusted Linux execution controller for untrusted
// C++17 submissions.
//
// Docker is invoked only from this process. Student compilation and
// execution happen in containers created from the fixed image under
// apps/backend/sandbox/cpp17. The controller never enables CGO, never
// mounts the host workspace or the Docker socket, and never forwards the
// process environment into a student container.
//
// Task 3.5 owns student verdict mapping. Observation records the raw
// signals that mapping will consume; this package does not assign AC, WA,
// TLE, CE, RE, or MLE.
package sandbox
