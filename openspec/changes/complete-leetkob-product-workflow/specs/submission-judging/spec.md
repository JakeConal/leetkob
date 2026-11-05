# Spec Delta

## Purpose

Defines reliable acceptance and isolated judging of C++ submissions while preserving deadline fairness and selecting exactly the work that counts toward each student's grade.

## ADDED Requirements

### Requirement: Submission acceptance window
Enrolled students SHALL submit pasted C++ or a `.cpp` file of at most 65,536 UTF-8 bytes while `opensAt <= receivedAt < submissionDeadline`. The authoritative receipt time SHALL be assigned by the server after the complete bounded request arrives and before queue processing. Invalid or oversized requests SHALL NOT count as submissions.

#### Scenario: Last-moment submission waits in the queue
- **WHEN** a complete valid request arrives before the deadline but finishes judging after it
- **THEN** it remains eligible for the counted-submission selection

#### Scenario: Request arrives at the deadline
- **WHEN** the completed request's server receipt time equals or exceeds the deadline
- **THEN** the request is rejected and cannot replace the student's previous counted submission

### Requirement: Idempotent durable acceptance
Submission creation SHALL bind an idempotency key to the authenticated student, lab, and request fingerprint. A successful response SHALL mean the source and processing intent are durable. Same-key retries SHALL return the original submission; different content with that key SHALL be rejected.

#### Scenario: Connection drops after acceptance
- **WHEN** the student retries the same request after the deadline using the original key
- **THEN** the server returns the already accepted submission without creating a late or duplicate submission

#### Scenario: Creation fails before durability
- **WHEN** source storage or the submission transaction fails
- **THEN** the system does not acknowledge successful acceptance or replace the counted submission

### Requirement: Counted submission selection
The system SHALL count the last valid submission received before the deadline, regardless of its judge verdict. Before the deadline it SHALL provisionally count the latest submission. Ties in server receipt time SHALL use a durable server-assigned order; client timestamps and worker completion order SHALL NOT affect selection.

#### Scenario: Later submission is wrong
- **WHEN** an Accepted submission is followed by a later valid Wrong Answer submission before the deadline
- **THEN** the later submission counts, receives a default zero, and the earlier submission's findings leave the active review queue

#### Scenario: Older worker finishes last
- **WHEN** a worker completes an older submission after the newer counted submission
- **THEN** the older result remains historical and cannot replace the counted result, findings, or grade

### Requirement: Durable job processing
Accepted submissions SHALL be judged asynchronously with recoverable processing state. Duplicate delivery and worker restarts SHALL NOT duplicate results or downstream effects. Exhausted infrastructure retries SHALL remain visible as an operational failure that blocks readiness, rather than becoming a student test failure or disappearing.

#### Scenario: Worker dies during judging
- **WHEN** a worker loses its processing lease before recording a result
- **THEN** another worker can retry and only the current authorized attempt can commit the result

#### Scenario: Result delivery is duplicated
- **WHEN** the same completed judge event is delivered twice
- **THEN** only one result revision and one eligible constraint-checking request are recorded

### Requirement: Isolated compilation and execution
Both compilation and test execution SHALL enforce the lab limits without network access, root execution, host-directory mounts, Docker socket access, or credentials. Each run SHALL use disposable isolated files and SHALL NOT access other submissions. Every isolation requirement SHALL have a recorded attack case demonstrating rejection or containment.

#### Scenario: Submission attempts escape or exfiltration
- **WHEN** submitted code probes networking, host paths, process limits, other runs, privilege escalation, or service credentials
- **THEN** the attempted access is denied or contained and the host and other submissions remain inaccessible

#### Scenario: Worker provides execution input
- **WHEN** a submission is compiled or tested
- **THEN** only the compiler/runtime and staged task files are available inside its execution boundary, without inherited API, database, storage, or model credentials

### Requirement: Judge verdicts and limits
The judge SHALL return aggregate and per-test verdicts using AC, WA, TLE, MLE, RE, and CE. Outputs SHALL be compared token by token ignoring whitespace. Compilation failure or timeout SHALL yield CE; CPU or wall timeout TLE; memory exhaustion MLE or documented RE; output overflow RE. AC SHALL require all tests to pass.

#### Scenario: Output differs only in whitespace
- **WHEN** all output tokens match the expected tokens within resource limits
- **THEN** the test is Accepted

#### Scenario: A process exceeds a fixed limit
- **WHEN** the program emits over 1 MiB for a test or attempts to exceed 16 processes including threads
- **THEN** output overflow yields RE and further process creation is denied, with the eventual verdict determined by the program's resulting behavior

### Requirement: Private realtime results and history
Students SHALL receive their own aggregate and per-test judge status as it becomes available, without hidden test contents or review findings. Reconnection SHALL recover current authoritative state. The enhancement phase SHALL provide the student's complete submission history with the counted submission clearly identified.

#### Scenario: Browser reconnects
- **WHEN** a student reconnects after missing completion events
- **THEN** the client fetches current results and shows the correct counted submission without duplicate or reordered updates

### Requirement: Versioned rejudging
Before publication, the enhancement phase SHALL allow an assigned TA to revise tests and rejudge a submission or lab. Results SHALL identify their test revision; affected findings and decisions SHALL be invalidated. Published labs SHALL reject bulk test changes and rejudging under this baseline, using individual grade corrections instead.

#### Scenario: Old test run completes after a revision
- **WHEN** a result for a superseded test revision arrives
- **THEN** it is retained as history but cannot satisfy current judging or publication readiness
