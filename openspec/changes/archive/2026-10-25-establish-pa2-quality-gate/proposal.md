# Proposal

## Why

LeeTKOB needs a trustworthy engineering baseline before later milestones add sandboxed execution and model-assisted constraint review. PA#2 specifically requires a reproducible local harness, a CI gate that demonstrably blocks contract regressions, and a client-ready contract for a high-risk endpoint.

## What Changes

- Establish a Node.js/TypeScript workspace with a NestJS API on Fastify and a Next.js App Router frontend using TanStack Query, while keeping one-command lint, format check, type check, and test workflows identical locally and in CI.
- Add concise repository rules covering the stack, commands, conventions, product-specific prohibitions, and three-tier assistant permissions.
- Document environment variables without values, keep secrets out of version control, and document the capped model budget policy.
- Specify and implement `POST /labs/{labId}/publish`, including TA role and class ownership checks, submission-deadline and pending-review refusal, RFC 9457 errors, idempotency, and publication state changes.
- Add contract tests for the success path, every documented error type, ownership, and idempotent retries.
- Add a responsive reviewer dashboard that exposes API health and the publication gate without disclosing student data, source code, or credentials.
- Require all quality checks on pull requests to `main`, demonstrate a blocked contract-breaking change, then record the repair and CI evidence.
- Add the PA#2 evidence index, AI usage log, self-assessment report, submission tag, and submission archive.

## Capabilities

### New Capabilities

- `lab-publishing`: Defines who may publish a lab, the preconditions, idempotent behavior, response representation, state transition, and problem-details errors.
- `quality-gate`: Defines the reproducible quality commands, pull-request CI behavior, required checks, secret-safety baseline, and evidence needed to prove the gate rejects a contract regression.
- `review-dashboard`: Defines the reviewer-facing Next.js surface, its TanStack Query API-health boundary, publication status presentation, and safe loading and failure states.

### Modified Capabilities

None.

## Impact

- Adds an npm-workspace modular monolith with a NestJS/Fastify API, Next.js frontend, in-memory PA#2 domain model, and contract test suite.
- Adds the framework runtime and development dependencies plus GitHub Actions workflow configuration.
- Adds repository governance and delivery files including `AGENTS.md`, `.env.example`, `README.md`, `PA2.md`, `AI-LOG.md`, and `SELF_ASSESSMENT_REPORT.md`.
- Changes GitHub repository settings by protecting `main`, requiring quality checks, and using pull requests for all later changes.
