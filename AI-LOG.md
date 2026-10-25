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

### D8 — Changed: resolve optional SonarCloud findings

- **Initial output:** every repository-controlled CI job passed, but optional SonarCloud analysis reported one security and seven maintainability findings.
- **Changed:** CI installs with `npm ci --ignore-scripts`, reduced-motion CSS no longer relies on `!important`, and query-state rendering uses explicit branches instead of nested ternaries.
- **Why:** the findings were substantiated and could be fixed without weakening a test, changing the contract, or altering the rendered states.
- **Evidence:** `.github/workflows/quality.yml`, `apps/web/app/globals.css`, and `apps/web/components/system-status.tsx`.

### D9 — Rejected: merge an intentionally broken publication rule

- **Suggestion considered:** treat the deliberate regression as evidence once the Test job failed, then move on without repairing the same pull request.
- **Rejected:** the contract and tests stayed unchanged, the implementation was repaired in the same protected pull request, and merge waited for all required checks to return green.
- **Why:** a red check proves the gate can detect a violation only if GitHub also prevents that revision from entering `main`.
- **Evidence:** [PR #2](https://github.com/JakeConal/leetkob/pull/2), failing commit [`4e808f7`](https://github.com/JakeConal/leetkob/commit/4e808f7a96d46c0c67d794b5f452866162ace116), failing [Test job](https://github.com/JakeConal/leetkob/actions/runs/40078757471/job/120827694233), repair commit [`bf6cb02`](https://github.com/JakeConal/leetkob/commit/bf6cb02cc6d0b5e336de97fb121b9060000382cc), and green [run 40078849258](https://github.com/JakeConal/leetkob/actions/runs/40078849258).

### D10 — Changed: derive required checks before enabling protection

- **Initial output:** the workflow existed, but the required-check names had not yet been observed from a real GitHub run.
- **Changed:** established a green baseline, read the emitted check contexts, then protected `main` with strict `Lint`, `Format`, `Typecheck`, and `Test` contexts plus administrator enforcement and pull-request flow.
- **Why:** GitHub branch protection matches status contexts by exact name; guessing could leave a required command unenforced or make every pull request unmergeable.
- **Evidence:** [`docs/evidence/main-branch-protection.json`](docs/evidence/main-branch-protection.json), baseline [run 40053375307](https://github.com/JakeConal/leetkob/actions/runs/40053375307), and protected merge commits [`1706076`](https://github.com/JakeConal/leetkob/commit/1706076781afafb30d443e7aaf20345fa8cd9eed) and [`fd74035`](https://github.com/JakeConal/leetkob/commit/fd74035535e83049665692953cf16a6aaf47cbaa).

### D11 — Changed: add the product-owner deadline gate

- **Initial output:** publication required an empty review queue but did not model the lab's submission deadline.
- **Changed:** after the user approved the externally visible contract update, the lab model gained an RFC 3339 deadline and the publication service returns `409 lab-still-open` before checking reviews when server time is earlier. The rejection participates in exact-response idempotency.
- **Why:** [product-owner FAQ-12 at immutable commit `2b82b90`](https://github.com/tkob-team/LeeTKOB-FAQ/blob/2b82b90ca3696fa81c68464f8a51ceb86406fef6/FAQ.md#faq-12) requires both the submission deadline to have passed and every queue item to have a TA decision before whole-lab publication.
- **Evidence:** `docs/contract/publish-lab.md`, `apps/api/src/domain/publication-service.ts`, and the deadline cases in both publication test suites.

### D12 — Changed: make rubric evidence publicly auditable

- **Initial state:** the GitHub repository was private, so the independent grader capped the current result at 97/100 and external evidence links could not be audited anonymously.
- **Changed:** after explicit user approval, changed `JakeConal/leetkob` to public while preserving protected `main` and its required checks.
- **Why:** the PA#2 hand-in rules require a public repository, and public evidence is necessary for the lecturer and the later red team.
- **Evidence:** <https://api.github.com/repos/JakeConal/leetkob> reports `visibility: public`; protected settings remain captured in `docs/evidence/main-branch-protection.json`.

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

### 2026-10-24 — Protected gate demonstration

- Opened PR #2 with an implementation-only change that bypassed the pending-review guard; the unchanged contract test failed and GitHub reported the pull request as blocked.
- Restored the guard in the same pull request without editing the contract, test, expected result, or label.
- Waited for all repository checks and SonarCloud to pass, then merged through protected `main`.

### 2026-10-25 — Product-owner FAQ alignment

- Reviewed the owner FAQ and identified that FAQ-12 added a deadline precondition missing from the original publication contract.
- Amended OpenSpec and the standalone contract before implementation, then added domain and HTTP contract cases for ordering, exact-deadline behavior, state safety, and replay; the domain case demonstrated the original mismatch before the implementation changed.
- Added the injected-clock deadline guard and exposed both publication preconditions on the theme-driven dashboard.

### 2026-10-25 — Final grading and hand-in verification

- Ran a fresh lockfile install and all five local rubric checks; 36 tests passed, the reachable-history credential scan passed, and the dependency audit reported zero vulnerabilities.
- Asked an independent grading agent to rescore the public repository; it reported 100/100 with no substantiated rubric deduction and separately identified tag/archive completion as the remaining hand-in operation.
- Pushed annotated tag `pa2`, waited for green tag run 40152904321, and created `23120124_100.zip` from that tag with complete Git metadata.
- Extracted the zip into a clean directory and verified `.git`, `PA2.md`, `AI-LOG.md`, `SELF_ASSESSMENT_REPORT.md`, `README.md`, the lockfile, a clean Git status, and matching `HEAD`/`pa2` commit IDs.

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
