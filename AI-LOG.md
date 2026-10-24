# AI Log

## Tools

- **Codex**: requirements analysis, OpenSpec planning, implementation, tests, documentation, CI, and review.
- **OpenSpec 1.14.3**: proposal, capability specifications, design decisions, tracked tasks, and verification.
- **Google Classroom / Drive reads**: source PA#2 rubric and LeeTKOB class brief.

## Decisions

### D1 — Changed: deterministic tests only

- **Suggestion considered:** wire a real model provider early so tests resemble the later PA#3 flow.
- **Changed:** PA#2 has no model integration. Unit and contract tests use only local deterministic inputs and are prohibited from making real model calls.
- **Why:** a live provider makes tests nondeterministic, spends money, and risks exposing a model key.
- **Evidence:** `AGENTS.md` Never section and the test suite.

### D2 — Rejected: trusted role headers

- **Suggestion considered:** use `X-Role` and `X-Class-Ids` headers to simplify contract tests.
- **Rejected:** the HTTP boundary verifies an HMAC-signed bearer claim instead.
- **Why:** trusted role headers let any client forge TA access and invalidate the ownership test.
- **Evidence:** `AGENTS.md` Never section and `src/auth/token.ts`.

### D3 — Changed: correlate response type with its body

- **Initial output:** represented every stored response with one union body unrelated to its content type.
- **Changed:** made `StoredResponse` a discriminated union: JSON success always carries Publication, and problem JSON always carries Problem Details.
- **Why:** `npm run typecheck` correctly rejected contract tests that accessed problem fields without proof that the response was a problem.
- **Evidence:** `src/domain/types.ts` and the `assertProblem` checks in `test/domain/publication-service.test.ts`.

### D4 — Rejected: broad keyword secret scanning

- **Suggestion considered:** fail on generic words such as `token`, `password`, or `secret` anywhere in the repository.
- **Rejected:** the scanner checks only high-confidence credential shapes and private-key markers, scans reachable Git blobs, and reports locations without matched values.
- **Why:** broad keywords would flag documentation and security tests while adding little evidence that a live credential exists.
- **Evidence:** `scripts/scan-secrets.mjs`; its temporary synthetic credential fixture was rejected while the repository passed.

## Session record

### 2026-10-24 — PA#2 planning and baseline

- Retrieved the PA#2 rubric and class brief, then selected `POST /labs/{labId}/publish` as the highest-risk contract.
- Created and strictly validated OpenSpec change `establish-pa2-quality-gate`.
- Chose Node's built-in HTTP stack to keep PA#2 at zero runtime dependencies.
- Implemented signed bearer verification, in-memory ports/adapters, serialized publication state transitions, and the HTTP adapter.
- Added a five-job GitHub Actions quality gate and ran all five commands locally; 28 tests passed and the dependency audit found no vulnerability.

## Changed

- Replaced generic agent guidance with LeeTKOB-specific prohibitions and approval tiers.
- Required exact-response idempotency for both success and deterministic client errors.
- Replaced an uncorrelated response-body type after the type checker exposed unsafe problem-field access.
- Added a bounded, high-confidence secret scan over the working tree and reachable history.

## Rejected

- Rejected a production database for PA#2; storage stays behind an interface for replacement in PA#3.
- Rejected status-code-only tests; contract tests must assert problem types and response fields.
- Rejected a web framework at this stage; Node's built-in HTTP server keeps runtime dependencies at zero and still exercises the network boundary.
- Rejected logging unexpected exception details; the `internal-error` response is fixed and generic.
