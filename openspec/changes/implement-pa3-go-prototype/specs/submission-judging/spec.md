# Spec Delta

## Purpose

Accept student C++ submissions durably and judge them asynchronously in isolation, with reproducible recovery and containment evidence.

## ADDED Requirements

### Requirement: Durable eligible submission acceptance

A student SHALL submit only to an enrolled open lab. The full bounded source request SHALL arrive before the deadline. Acknowledgement SHALL follow durable source, counted-selection and queue-intent commit. Same-key/content retries SHALL return the original result, including after the deadline; changed content SHALL conflict.

#### Scenario: PA3-JUDGE-01 Submission is queued

- **WHEN** a valid bounded submission is accepted while the worker is stopped
- **THEN** its queued state survives API restart and the response identifies a durable submission

#### Scenario: PA3-JUDGE-02 Deadline and retry boundaries

- **WHEN** a new request completes exactly at the deadline or an accepted request is retried after it
- **THEN** the new request is rejected and the matching retry replays without creating a duplicate

### Requirement: Separate recoverable worker

A separate worker SHALL lease queued work and store exactly one set of verdicts per submission/input revision. Worker restart or duplicate execution SHALL NOT duplicate stored effects. Expired attempts SHALL be fenced from committing. Exhausted infrastructure errors SHALL remain visible rather than becoming student failures.

#### Scenario: PA3-JUDGE-03 Stop submit restart

- **WHEN** the worker is stopped, a submission is accepted, and the worker is restarted
- **THEN** the submission reaches judged status with exactly one stored verdict set, proven by a test or recording

#### Scenario: PA3-JUDGE-04 Late worker attempt

- **WHEN** a worker loses its lease and later attempts to store a result after a replacement worker
- **THEN** its result cannot replace or duplicate the committed current result

### Requirement: Nine execution minimums

Compilation and execution SHALL have no network; bounded CPU; a wall timeout that kills the container; memory bounded including swap; a process limit; bounded output; a non-root user; no host mounts or Docker socket; and no secrets in environment, files or compiler-includable paths. Each minimum SHALL have an attack submission exercised by CI on the pa3 tag.

#### Scenario: PA3-JUDGE-05 Attack suite runs

- **WHEN** the tag-CI attacks attempt network, CPU, wall, memory, process, output, privilege, host/socket and secret access
- **THEN** each is denied or contained with recorded configuration and observed results; the memory case reads allocations back and the wall case proves container termination

#### Scenario: PA3-JUDGE-06 Compilation attempts secret inclusion

- **WHEN** untrusted source uses preprocessing or compiler-time code to probe credential files or service environment
- **THEN** no model key or other service secret is visible to that compilation boundary

### Requirement: Correct private judge results

The judge SHALL compile C++17, compare output tokens, and produce aggregate/per-test AC, WA, TLE, CE or RE, with MLE where distinguished and memory RE documented otherwise. Students SHALL see only their own judge status, never hidden I/O or checker findings. Only current counted AC work SHALL proceed to constraint checks.

#### Scenario: PA3-JUDGE-07 Marking submissions progress

- **WHEN** the compliant, violating and injection solutions are submitted through the real ingress
- **THEN** each progresses from queued to judged, the student can observe the result, and eligible AC work reaches checking

#### Scenario: PA3-JUDGE-08 Later WA replaces earlier AC

- **WHEN** a later valid submission is WA after an earlier AC and the earlier worker finishes last
- **THEN** the later receipt remains counted and the old AC findings cannot reenter the active queue
