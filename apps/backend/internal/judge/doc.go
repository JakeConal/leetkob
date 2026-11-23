// Package judge compiles and executes one submission in the sandbox and
// maps the controller's raw observations to student verdicts.
//
// MemoryRENote is the MLE versus memory-RE rule. The sandbox records
// Docker State.OOMKilled on Observation.OOMKilled. That bit is the cgroup
// memory killer, which is distinct from the SIGKILL this controller sends
// when the wall deadline fires. An execution observation with OOMKilled
// set is MLE.
//
// A backend that cannot report that bit must not call this mapping for a
// memory death. Without the bit, memory exhaustion is RE (memory RE). A
// signal exit with OOMKilled, WallTimedOut, and OutputLimited all false is
// the CPU-time death and is TLE, not MLE and not RE. Compilation failure,
// including a compilation wall timeout or a compilation OOM, is CE.
// Output overflow and any other non-zero execution exit are RE. Token
// comparison decides AC or WA only after a clean exit. An error returned
// by Compile or ExecuteInput is an infrastructure failure and is never
// stored as a student verdict.
package judge

// MemoryRENote documents why this package emits MLE for execution OOM and
// when a caller must record memory RE instead.
const MemoryRENote = "Execution OOMKilled maps to MLE because the sandbox records State.OOMKilled, distinct from the wall-clock SIGKILL. A backend that cannot report that bit must document the memory result as RE and must not use this mapping for that death. A signal exit without OOMKilled, WallTimedOut, and OutputLimited is TLE."
