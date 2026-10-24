# AI Log

## Tools

- **Codex**: requirements analysis, OpenSpec planning, implementation, tests, documentation, CI, and review.
- **OpenSpec 1.14.3**: proposal, capability specifications, design decisions, tracked tasks, and verification.
- **Google Classroom / Drive reads**: source PA#2 rubric and LeeTKOB class brief.
- **Image generation**: early full-screen dashboard concept; later superseded by the user-provided repository theme.
- **Vercel React guidance**: server/client boundary, bundle discipline, and query-state implementation review.

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
- **Evidence:** `AGENTS.md` Never section and `apps/api/src/auth/token.ts`.

### D3 — Changed: correlate response type with its body

- **Initial output:** represented every stored response with one union body unrelated to its content type.
- **Changed:** made `StoredResponse` a discriminated union: JSON success always carries Publication, and problem JSON always carries Problem Details.
- **Why:** `npm run typecheck` correctly rejected contract tests that accessed problem fields without proof that the response was a problem.
- **Evidence:** `apps/api/src/domain/types.ts` and the `assertProblem` checks in `apps/api/test/domain/publication-service.test.ts`.

### D4 — Rejected: broad keyword secret scanning

- **Suggestion considered:** fail on generic words such as `token`, `password`, or `secret` anywhere in the repository.
- **Rejected:** the scanner checks only high-confidence credential shapes and private-key markers, scans reachable Git blobs, and reports locations without matched values.
- **Why:** broad keywords would flag documentation and security tests while adding little evidence that a live credential exists.
- **Evidence:** `scripts/scan-secrets.mjs`; its temporary synthetic credential fixture was rejected while the repository passed.

### D5 — Changed: modern framework workspace

- **Initial output:** the green baseline used Node's built-in HTTP server and no runtime dependencies.
- **Changed:** after explicit user approval, migrated to npm workspaces with NestJS 12 on Fastify for the API and Next.js 16 App Router with TanStack Query for the web application.
- **Why:** the chosen combination provides a structured backend boundary and a current frontend architecture while preserving one clone, lockfile, and quality gate.
- **Evidence:** `apps/api/package.json`, `apps/web/package.json`, and OpenSpec decision “npm workspaces with NestJS/Fastify and Next.js”.

### D6 — Rejected: TanStack Router beside Next.js

- **Suggestion considered:** use TanStack for both queries and routing.
- **Rejected:** Next.js App Router already owns application routing; only TanStack Query is used for API server state.
- **Why:** two routers would duplicate responsibility and add complexity without satisfying another PA#2 requirement.
- **Evidence:** `apps/web/app` and `apps/web/components/system-status.tsx`.

### D7 — Changed: repository theme overrides concept styling

- **Initial output:** the dashboard concept used a conventional blue, amber, and green SaaS palette.
- **Changed:** applied `docs/design/theme.md` as the authoritative source for the Swiss Minimalist grid, typography, square controls, exposed rules, and restrained red signaling while preserving the concept's information hierarchy.
- **Why:** the user explicitly selected the repository theme after the layout concept was generated.
- **Evidence:** `apps/web/app/globals.css` and `docs/design/theme.md`.

## Session record

### 2026-10-24 — PA#2 planning and baseline

- Retrieved the PA#2 rubric and class brief, then selected `POST /labs/{labId}/publish` as the highest-risk contract.
- Created and strictly validated OpenSpec change `establish-pa2-quality-gate`.
- Chose Node's built-in HTTP stack to keep PA#2 at zero runtime dependencies.
- Implemented signed bearer verification, in-memory ports/adapters, serialized publication state transitions, and the HTTP adapter.
- Added a five-job GitHub Actions quality gate and ran all five commands locally; 28 tests passed and the dependency audit found no vulnerability.

### 2026-10-24 — Framework and dashboard migration

- Migrated the OpenSpec before implementation and added the `review-dashboard` capability.
- Moved the unchanged domain, authentication, and in-memory adapters into `apps/api`.
- Replaced the raw HTTP edge with NestJS controllers, a Fastify bootstrap, and a global RFC 9457 exception filter while preserving publication contract behavior.
- Added the responsive Next.js dashboard, isolated TanStack Query to API health, and added frontend health-contract tests.
- Applied the user-provided `theme.md` as the complete UI source of truth and removed the superseded concept asset from the project.

## Changed

- Replaced generic agent guidance with LeeTKOB-specific prohibitions and approval tiers.
- Required exact-response idempotency for both success and deterministic client errors.
- Replaced an uncorrelated response-body type after the type checker exposed unsafe problem-field access.
- Added a bounded, high-confidence secret scan over the working tree and reachable history.

## Rejected

- Rejected a production database for PA#2; storage stays behind an interface for replacement in PA#3.
- Rejected status-code-only tests; contract tests must assert problem types and response fields.
- Initially rejected a web framework before it was requested; the later user-approved migration is recorded in D5 and keeps the same domain and contract boundaries.
- Rejected adding TanStack Router alongside Next.js App Router.
- Rejected logging unexpected exception details; the `internal-error` response is fixed and generic.
