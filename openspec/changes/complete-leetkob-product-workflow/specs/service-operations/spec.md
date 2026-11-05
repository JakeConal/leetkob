# Spec Delta

## Purpose

Defines the operational evidence and recovery behavior needed to run LeeTKOB reliably for a real class and support review, red-team testing, and course demonstrations.

## ADDED Requirements

### Requirement: Correlated privacy-preserving telemetry
The service SHALL emit structured logs, metrics, and traces correlating submission acceptance, queue processing, judging, checking, review, publication, and appeals. Operational telemetry SHALL exclude credentials, submitted source, hidden test data, and raw model prompts or responses.

#### Scenario: Operator diagnoses slow processing
- **WHEN** a submission takes longer than expected
- **THEN** authorized operational views connect its processing stages through opaque correlation IDs without revealing its source or student identity

### Requirement: Operational dashboard and alerts
An authorized operations dashboard SHALL show queue depth and age, judge durations, model latency and failures, token costs and budget reservations, unsure rate, and TA rejection rate. Documented thresholds SHALL detect stalled queues, unavailable workers, provider failures, and exhausted budgets.

#### Scenario: Model provider becomes unavailable
- **WHEN** provider failures cross the documented alert threshold
- **THEN** operators see the failure and affected labs while students retain access to judge results and TAs can resolve unsure findings manually

### Requirement: Recoverable durable state
Restarting API or worker processes SHALL preserve accepted submissions, jobs, current review decisions, grades, appeals, audit history, and completed idempotency responses. Recovery SHALL reconcile expired leases and abandoned work without duplicating business effects. Migration and restore procedures SHALL be documented and exercised before release.

#### Scenario: Service restarts after publication
- **WHEN** the service restarts and a caller retries a completed publication request
- **THEN** it returns the stored result under the applicable authorization rules and no duplicate publication or grade version is created

### Requirement: Measured eighty-submission workload
Release evidence SHALL include a reproducible workload of 80 simultaneous valid submissions near a deadline, with zero lost accepted submissions and zero duplicate counted submissions. The report SHALL identify hardware, limits, test corpus, worker concurrency, errors, throughput, queue delay, and p50/p95 acceptance and completion latencies.

#### Scenario: Submission burst spans deadline processing
- **WHEN** 80 requests are fully received before the deadline and queued work continues after it
- **THEN** all acknowledged submissions remain eligible, converge to results or explicit recoverable operational failures, and appear in the report without selective exclusion

### Requirement: Public deployable service and runbook
The release SHALL provide an approved public deployment reachable by the lecturer and red team, authenticated test accounts with synthetic data, and a reproducible runbook covering setup, migrations, configuration names, service health, job recovery, rollback, and restore. External deployment SHALL require the repository owner's authorization.

#### Scenario: Maintainer follows the runbook
- **WHEN** a maintainer starts from a clean supported environment with separately provisioned configuration
- **THEN** the documented procedure brings up the application and execution service and demonstrates the principal TA and student flow

### Requirement: End-to-end and red-team evidence
Release acceptance SHALL exercise admin setup, TA authoring, student submission, sandbox judging, checking, TA review, blocked and successful publication, student results, and both appeal outcomes. Red-team evidence SHALL document authorization and sandbox attacks, observed outcomes, remediation, and retests without exposing real student data.

#### Scenario: Unauthorized participant attempts publication
- **WHEN** a student or a TA from another class attempts to publish, read evidence, or access another user's event stream
- **THEN** the attempt fails and the release evidence records the tested boundary

### Requirement: Incident post-mortem
The release SHALL include a post-mortem for one real or explicitly labelled injected incident, with impact, timeline, detection, contributing causes, recovery, and actionable follow-up. A simulated exercise SHALL never be represented as an actual production incident.

#### Scenario: Worker crash exercise is documented
- **WHEN** an injected worker crash delays a lab
- **THEN** the report identifies it as an exercise and links recovery evidence and follow-up ownership
