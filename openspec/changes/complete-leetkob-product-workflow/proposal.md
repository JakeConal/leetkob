# Proposal

## Why

LeeTKOB currently demonstrates a safe publication contract and a reviewer shell, but cannot yet take a real class from lab creation through C++ submission, evidence-based review, published grades, and appeals. This change plans the remaining product and operational capabilities as staged deliverables, preserving the PA#2 baseline while completing the workflow described by the product-owner FAQ and detailed proposal.

## What Changes

- Add durable accounts, class membership, lab authoring, versioned constraints and tests, submissions, and student-facing results.
- Run C++17 judging in an isolated execution service with durable job recovery, then check counted Accepted submissions with syntax-aware rules and a guarded model adapter.
- Replace demonstration-only review data with authorized review queues, evidence, explicit TA decisions, pass sampling, grades, and audit history.
- Add a production publication API with processing-readiness checks and atomic grade visibility, then individual corrections and seven-day deduction appeals. Preserve the unversioned PA#2 publication contract; route production clients through a separately documented `/v2/labs/{labId}/publish` contract.
- Add reproducible checker evaluation, operational telemetry, 80-submission load evidence, deployment/runbook material, red-team findings, and an incident post-mortem.
- Include the proposal's 13 Should features as a final enhancement phase. Account for all 57 feature IDs in the design; retain completed PA#2 work and explicitly defer the seven Could features. Rechecking changed constraints is mandatory under FAQ-07 even though proposal E6 is labelled Should.

## Capabilities

### New Capabilities

- `identity-and-classes`: Account lifecycle, verified identity, roles, class ownership, enrollment, and roster management.
- `lab-authoring`: Lab schedules, statements, test bundles, constraint revisions, execution limits, reuse, and reference-solution checks.
- `submission-judging`: Deadline-aware acceptance, counted submissions, durable jobs, sandbox execution, results, and rejudging.
- `constraint-checking`: Sanitized inputs, deterministic rules, semantic model checks, exact evidence, revisions, and cost limits.
- `review-decisions`: Required queue selection, pass sampling, TA review, overrides, and audit trails.
- `grading-and-appeals`: Deterministic grades, student visibility, post-publication corrections, deduction appeals, exports, and notifications.
- `checker-evaluation`: Dataset-independent checker CLI, official scoring thresholds, team cases, and revision-bound evaluation evidence.
- `service-operations`: Telemetry, recovery, deployment, load testing, operational documentation, and acceptance evidence.

### Modified Capabilities

- `lab-publishing`: Add a versioned production operation with durable readiness, current authorization, immutable publication metadata, and atomic visibility of complete grades; preserve the PA#2 operation.
- `review-dashboard`: Add protected live review and publication flows while retaining a clearly separated safe demonstration mode and existing health behavior.
- `quality-gate`: Extend the existing checks to all workers and contracts, and require trustworthy checker-evaluation evidence for checker-affecting revisions.

## Impact

Implementation will extend `apps/api` and `apps/web`, introduce worker/checker packages, database migrations and adapters, execution infrastructure, new contract documents, and operational/evaluation tooling. PostgreSQL is the proposed durable store and job source; isolated Linux C++ execution and a syntax parser are additional runtime boundaries requiring dependency approval before installation. Model/provider selection stays configurable and retains the repository's USD 0.25 per-lab cap.

Sources are [FAQ-01 through FAQ-14](https://github.com/tkob-team/LeeTKOB-FAQ/blob/main/FAQ.md), the [detailed proposal](https://github.com/tkob-team/proposal-AWAD/blob/main/docs/proposal-detail.md), and the [official dev-set contract](https://github.com/fithcmus/awad-leetkob-eval/blob/main/README.md). The class brief takes precedence over these sources but was not supplied in this planning session; the design identifies the limited acceptance details that must be reconciled before release. This proposal does not authorize paid requests, infrastructure changes, deployment, or coursework submission.
