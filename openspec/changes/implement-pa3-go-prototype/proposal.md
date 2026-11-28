# Proposal

## Why

LeeTKOB currently has the PA#2 publication contract, a demonstration dashboard and pinned evaluation-source tooling, but cannot execute the flow assessed by PA#3. Deliver the Go prototype against the now-published eight-criterion rubric by November 29, 2026 at 22:00 Asia/Saigon, with reviewable evidence instead of treating the whole remaining product as one milestone.

## What Changes

- Add Go API, worker and checker entry points, durable state and signed-in synthetic test accounts with current class authorization. Preserve PA#2 contract behavior and tests in isolated compatibility mode.
- Let a TA create the marked lab, tests and constraints through real screens/API; let enrolled students submit C++ and see only their own judge status.
- Queue work durably for a separate worker; isolate both compilation and execution and prove all nine sandbox minimums with tag-CI attack submissions and restart/deduplication evidence.
- Run generic syntax-aware hard rules before real semantic model checks. Sanitize model inputs, enforce schema/evidence validation, timeout and the repository's USD 0.25 per-lab cap, and route findings to assigned TAs without publishing deductions.
- Provide the dataset-independent Docker checker with `MODEL_OFF=1`, an unchanged official dev set, at least five separate team cases, and a committed real-model dev run with predictions, report and provider call provenance.
- Add the PA#3 evidence index, first evaluation/cost arithmetic, focused test-first history, deployment/runbook material and rubric-based self-assessment. Public deployment and paid evaluation remain separately authorized release actions.

## Capabilities

### New Capabilities

- `identity-and-classes`: PA#3 test-account sign-in, revocable sessions and current class access.
- `lab-authoring`: TA-authored labs, explicit constraints/deductions, tests and execution limits.
- `submission-judging`: Durable admissions, recoverable worker jobs, isolated C++ judging and private results.
- `constraint-checking`: Syntax-aware bans, sanitized guarded semantic calls and bounded cost.
- `checker-evaluation`: Generic Docker checker, model-off operation and reproducible first evaluation.
- `review-decisions`: Private evidence queue and deliberate TA decisions with no automatic publication.

### Modified Capabilities

- `review-dashboard`: Add authenticated live authoring/submission/review views while preserving a clearly separate safe demonstration mode and disabled production publication.
- `quality-gate`: Cover Go commands and sandbox tag CI, preserve existing required checks, and add auditable PA#3 release evidence and focused history.

## Impact

Add `apps/backend` and extend the existing Next.js app, root check scripts, container configuration and documentation. Proposed dependencies are pgx/PostgreSQL, Argon2id and isolated LLVM/Clang/GCC tooling; the concrete inventory and API contract require owner approval before installation/implementation. No registration/JWT migration, appeals, grades, CSV import, SSE, telemetry platform or unrelated Should features are prerequisites for this checkpoint.

This change owns PA#3 delivery. `complete-leetkob-product-workflow` and issue #7 remain the broader roadmap; their overlapping capability deltas and tasks must be reconciled against delivered behavior before either change is archived. Creating this change does not complete the umbrella tasks, approve default-backend cutover, weaken its later requirements, authorize paid calls/deployment, or submit coursework.

Sources: [PA#3 assignment](https://classroom.google.com/c/ODg2MTU1OTQ4NDc0/a/ODk5OTQ1NDY0NTgy/details), [rubric](https://drive.google.com/file/d/1EGXZQNjbevJ5cE18rgXEujeKJ1hFeFvf/view), [source reconciliation](../../../docs/requirements/acceptance-matrix.md#pa3-published-rubric-and-implementation-deltas).
