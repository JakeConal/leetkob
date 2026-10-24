# Design

## Context

The repository currently contains only OpenSpec scaffolding and one setup commit. There is no starter application to preserve. The PA#2 rubric requires a small but real contract implementation, tests that exercise it, identical local/CI commands, protected-branch evidence, and a reproducible tagged submission. See `proposal.md` for motivation and the two delta specs for observable behavior.

The implementation must remain a useful foundation for PA#3 without pretending to provide the later sandbox, queue, model, database, or deployment milestones.

## Goals / Non-Goals

**Goals:**

- Produce a Node 22+ TypeScript service whose domain logic is independent of the HTTP adapter.
- Make every documented publication behavior directly contract-testable over HTTP.
- Keep state and authentication replaceable so PA#3 can add persistence and a real identity provider.
- Make local and GitHub-hosted quality results use exactly the same package scripts.
- Generate public, linkable evidence that branch protection and contract tests reject a regression.

**Non-Goals:**

- Durable storage, production authentication, deployment, sandboxed C++ execution, queues, model calls, eval data, or a user interface.
- Implementing endpoints other than the publication endpoint needed for this checkpoint.
- Submitting to Google Classroom; the user retains control of the final academic submission.

## Decisions

### Decision: Node 22+, strict TypeScript, and the built-in HTTP server

Use ESM TypeScript with strict compiler settings, Node's `http` module, Node's test runner, and built-in `fetch`. This keeps runtime dependencies at zero while still exercising a real network boundary. Development dependencies provide TypeScript execution, linting, and formatting.

Alternatives considered:

- Fastify or Express would reduce routing boilerplate, but adds runtime dependencies before the application needs a framework.
- A handler-only test would be faster, but would not prove header parsing, content type, serialization, and the actual route contract.

### Decision: Ports-and-adapters separation

Keep publication rules in a domain service that depends on repository, clock, and identity abstractions. Put parsing and RFC 9457 serialization in the HTTP adapter. The production entry point wires in-memory PA#2 adapters; tests create isolated fixtures and an ephemeral HTTP listener.

This lets a later milestone replace in-memory state without rewriting authorization, idempotency, or contract tests.

### Decision: Signed bearer claims for the PA#2 boundary

Use a minimal HMAC-signed bearer token containing subject, role, and assigned class IDs. The server requires `AUTH_TOKEN_SECRET`; tests inject an explicit non-production secret. The README labels this as a PA#2 boundary rather than a production identity system.

Alternatives considered:

- Trusting role headers would make authorization trivially forgeable and would undermine the ownership demonstration.
- A full OAuth/OIDC integration is outside PA#2 and would make fresh-clone testing depend on an external service.

### Decision: Exact-response idempotency ledger

Store a completed result under the tuple `(actorId, labId, idempotencyKey)` before returning it. A retry returns the stored status, safe headers, and body byte-for-byte. A new key against a published lab returns `lab-already-published`. The in-memory store performs the transition synchronously, avoiding partial writes in this milestone.

Alternatives considered:

- Treating an already-published lab as an unconditional success would hide accidental reuse of a new key and weaken client diagnostics.
- Caching only successful requests would allow a rejected retry to change meaning after mutable state changed, contradicting the specified deterministic-rejection behavior.

### Decision: One problem-type namespace

Use absolute problem type URIs under `https://leetkob.example/problems/` and a shared factory that guarantees the RFC 9457 core fields. Extension fields are narrowly typed: `pendingReviews` for `reviews-pending`, and publication metadata for `lab-already-published`.

### Decision: Four independent CI jobs plus a security audit

The workflow installs with `npm ci` and runs `npm run lint`, `npm run format:check`, `npm run typecheck`, and `npm test` as separately named jobs. A fifth audit job checks dependency metadata and scans the tree and reachable history for high-confidence credential patterns. Only the four rubric checks are configured as required branch checks; the security audit remains visible and must also stay green.

### Decision: Evidence-producing Git sequence

First land the complete green baseline and workflow on `main`, wait for its checks, then enable branch protection. After protection, create a branch with a deliberate contract regression only, open a pull request, capture the failing test run and blocked merge state, repair the implementation without editing the test or contract, wait for green, and merge through the pull request. Update evidence files through a second protected pull request if immutable URLs are not known until after merge.

### Decision: Academic records are factual and auditable

`AI-LOG.md` records the actual Codex/OpenSpec use, including concrete Changed and Rejected decisions. `SELF_ASSESSMENT_REPORT.md` claims only points supported by paths, commits, pull requests, settings, and CI URLs. `PA2.md` is the short index of those public artifacts.

## Risks / Trade-offs

- **[Risk] In-memory state is lost on restart.** → State durability is explicitly a PA#3 concern; isolate storage behind an interface and document the PA#2 limitation.
- **[Risk] Home-grown token verification could be mistaken for production auth.** → Keep it small, constant-time verify the signature, validate claims strictly, and label replacement with OIDC as required before deployment.
- **[Risk] GitHub check context names can differ from workflow labels.** → Run the workflow once, inspect the observed contexts, then configure branch protection with those exact names.
- **[Risk] The repository owner may be able to bypass protection.** → Configure administrator enforcement and verify the pull request reports blocked while the contract test is red.
- **[Risk] Evidence links cannot be finalized before runs and commits exist.** → Use an evidence-update pull request after the demonstration and tag only the final merged evidence revision.
- **[Risk] A naive secret scanner produces false positives.** → Limit patterns to high-confidence credential prefixes/private-key markers and manually review every match before the tag.
- **[Trade-off] Zero runtime dependencies means more HTTP plumbing.** → Keep the adapter narrow and covered by contract tests; reevaluate a framework when PA#3 adds endpoints and middleware.

## Migration Plan

1. Create and verify the application, documentation, tests, and CI workflow on the currently unprotected initial branch.
2. Push the green baseline and wait for all workflow jobs to succeed.
3. Enable `main` protection with the exact observed required checks and administrator enforcement.
4. Produce the deliberate failing pull request, verify it is blocked, repair it, and merge only after green.
5. Add final evidence links through protected pull-request flow, run all checks again, and merge.
6. Tag the final merge `pa2`, push the tag, and verify the tag workflow.
7. Create and verify the required zip archive including `.git` without committing the archive into the repository.

Rollback consists of reverting the affected pull request through the protected flow. Branch protection remains enabled; the `pa2` tag is created only after final verification.
