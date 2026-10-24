# Proposal

## Why

LeeTKOB needs a trustworthy engineering baseline before later milestones add sandboxed execution and model-assisted constraint review. PA#2 specifically requires a reproducible local harness, a CI gate that demonstrably blocks contract regressions, and a client-ready contract for a high-risk endpoint.

## What Changes

- Establish a Node.js/TypeScript service with one-command lint, format check, type check, and test workflows that are identical locally and in CI.
- Add concise repository rules covering the stack, commands, conventions, product-specific prohibitions, and three-tier assistant permissions.
- Document environment variables without values, keep secrets out of version control, and document the capped model budget policy.
- Specify and implement `POST /labs/{labId}/publish`, including TA role and class ownership checks, pending-review refusal, RFC 9457 errors, idempotency, and publication state changes.
- Add contract tests for the success path, every documented error type, ownership, and idempotent retries.
- Require all quality checks on pull requests to `main`, demonstrate a blocked contract-breaking change, then record the repair and CI evidence.
- Add the PA#2 evidence index, AI usage log, self-assessment report, submission tag, and submission archive.

## Capabilities

### New Capabilities

- `lab-publishing`: Defines who may publish a lab, the preconditions, idempotent behavior, response representation, state transition, and problem-details errors.
- `quality-gate`: Defines the reproducible quality commands, pull-request CI behavior, required checks, secret-safety baseline, and evidence needed to prove the gate rejects a contract regression.

### Modified Capabilities

None.

## Impact

- Adds the initial TypeScript service, in-memory PA#2 domain model, HTTP adapter, and contract test suite.
- Adds Node development dependencies and GitHub Actions workflow configuration.
- Adds repository governance and delivery files including `AGENTS.md`, `.env.example`, `README.md`, `PA2.md`, `AI-LOG.md`, and `SELF_ASSESSMENT_REPORT.md`.
- Changes GitHub repository settings by protecting `main`, requiring quality checks, and using pull requests for all later changes.
