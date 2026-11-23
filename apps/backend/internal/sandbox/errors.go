// Package sandbox compiles and executes one untrusted C++17 submission in
// a fixed Linux container, without network, root, host mounts, a Docker
// socket or service secrets.
//
// The package is the trusted controller side of PA#3 design decisions 5
// and 6: no submitted source is compiled or parsed on the trusted host,
// every compile and run happens inside the pinned image, and every
// externally visible failure is either a judging verdict or an explicit
// InfrastructureError — never a student verdict.
package sandbox

import (
	"errors"
	"fmt"
)

// InfrastructureError reports that the sandbox could not establish or
// observe the execution boundary at all (Docker unavailable, image
// missing, container start failure, inspection failure). Callers must
// surface it as an operational state and retry policy decision, never as
// a CE/TLE/RE/MLE verdict for the submission.
type InfrastructureError struct {
	// Op names the step that failed, for example "docker run" or
	// "docker inspect".
	Op string
	// Err is the underlying process, daemon or parse error.
	Err error
}

func (e *InfrastructureError) Error() string {
	if e.Op == "" {
		return "sandbox infrastructure failure: " + e.Err.Error()
	}
	return fmt.Sprintf("sandbox infrastructure failure (%s): %v", e.Op, e.Err)
}

// Unwrap supports errors.Is/errors.As against the underlying cause.
func (e *InfrastructureError) Unwrap() error { return e.Err }

// IsInfrastructure reports whether err represents an infrastructure
// failure rather than a verdict. It is the only supported way for the
// worker to decide between retrying and recording a judge result.
func IsInfrastructure(err error) bool {
	var target *InfrastructureError
	return errors.As(err, &target)
}

// ErrSourceTooLarge is returned before any container is started when the
// submission exceeds Config.SourceBytes (65536 bytes by default). It is a
// bounded-admission error, not an infrastructure failure and not a
// compile verdict.
var ErrSourceTooLarge = errors.New("sandbox: source exceeds the configured byte cap")

// ErrInvalidConfig is returned by New for a configuration that could
// weaken the boundary (for example a missing image or a non-positive
// limit). A zero value is never silently accepted.
var ErrInvalidConfig = errors.New("sandbox: invalid configuration")
