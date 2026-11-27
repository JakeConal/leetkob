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

### D13 — Changed: PA#3 dependency and API approval recorded; contract published

- **Initial state:** the PA#3 dependency inventory and API draft were awaiting owner approval, so no Go routes or modules could start.
- **Changed:** on November 23, 2026 the owner approved both documents as the PA#3 implementation boundary. The approved subset is published as OpenAPI 3.1 (`docs/contract/pa3-openapi.json`) with versioned internal schemas (`docs/contract/schemas/job-envelope.v1.json`, `checker-prediction.v1.json`) and exact RFC 9457 bodies (`docs/contract/fixtures/pa3-problems.json`) before any route exists.
- **Interpreted:** the approved error enumeration has exactly one 403 type, so CSRF/Origin mutation rejections reuse `role-required`; unsure causes are pinned as timeout, provider-failure, schema-invalid, evidence-invalid and sanitization-unsafe. Both interpretations are recorded here for review.
- **Why:** issue #38 task 1.1 requires recorded approval plus published contracts before new routes; paid-model access, public hosting and coursework upload remain separately authorized and are still undecided.
- **Evidence:** `docs/requirements/pa3-dependencies.md`, `docs/contract/pa3-api.md`, `docs/contract/pa3-openapi.json`, and this decision.

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

### 2026-11-05 — Reconcile class requirements for task 1.1

- Tool used was Codex with OpenSpec, authenticated Classroom/Drive readers, and GitHub source reads.
- Requested work was the cited acceptance matrix for the class brief and published checkpoint rubrics against the product-owner FAQ and proposal.
- Kept the brief's fixed thresholds, sandbox minimums, stack freedom, and milestone outcomes, with source versions and implementation-evidence obligations.
- Changed the source register after retrieving the original brief and PA#2 rubric. Identified PA#4's assigned-team red-team report as an earlier requirement than the current operations-phase schedule captures.
- Rejected treating binary units, precise time boundaries, conditional compilation timeout, or unpublished later rubrics as lecturer-confirmed details. Recorded those distinctions and the responsible follow-up reviews in `docs/requirements/acceptance-matrix.md`.

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

### 2026-11-09 — Evidence refresh before final hand-in

- Re-graded the repository against the PA#2 rubric with Claude Code and found an estimate of about 94–96. The team claim stays at **96**, which supersedes the 100/100 independent-agent estimate recorded on 2026-10-25.
- The submission archive is named `23120045-23120124-23120175_96.zip` (all three student IDs in ascending order and the claimed total). It replaces the earlier `23120124_100.zip` draft, which listed one ID and the outdated total.
- `PA2.md` and `SELF_ASSESSMENT_REPORT.md` linked tag runs from earlier tag positions (40152904321, 40177068435). They now link the filtered `pa2` run list, because a commit cannot contain the ID of the run its own tag push triggers.
- Re-captured `main` protection: the active ruleset now requires one approving review (previously 0) and has no bypass actors. The four required checks are unchanged.
- Aligned the install command in `AGENTS.md` with README and CI (`npm ci --ignore-scripts`).

### 2026-11-13 — Draft production contracts for task 1.3

- Tool used was Claude Code, reading the OpenSpec change, the acceptance matrix, and the pinned official evaluation README.
- Requested work was the language-neutral `/v2` OpenAPI schema, the versioned judge/checker envelopes, and contract fixtures before any implementation.
- Kept the v1.1 contract and its suite untouched. The nine shared problem types keep their URI, status, and title, and a test compares them with `docs/contract/publish-lab.md`.
- Rejected separate seconds and mebibyte members for execution limits. Both contracts use base units (`cpuTimeLimitMs`, `memoryLimitBytes`) so they share one representation and the binary-unit choice (D01) is explicit in the numbers.
- Changed the first test run's metadata after it failed: `setUserRole` takes `expectedRevision` but was annotated as not revision-checked, and `createSubmission` and `createAppeal` had no `invalid-request` scenario.
- Rejected a hand-written JSON Schema checker; added `yaml`, `ajv`, and `ajv-formats` as development-only dependencies of the API workspace instead. `ajv` and `ajv-formats` were already installed transitively.
- Rejected marking the task complete: contract review and the product-owner answer on D01 are still pending and are recorded as pending in the contract.
- Injected ten faults into the contract, fixtures, and envelope schema in turn (a shared problem status, a missing CSRF problem, a role member on registration, acceptance at the deadline, a dropped adjustment, inner-whitespace evidence, a changed replay, a leaking concealed 404, an open finding schema, a drifting table). Each failed the intended assertion and the files were restored byte for byte.

### 2026-11-20 — Separate PA#3 Go change

- Tools: Codex, authenticated Classroom/Drive and GitHub readers, local repository inspection, OpenSpec CLI and Prettier. Read the exact PA#3 assignment/rubric and current class brief; no model-provider request or deployment was made.
- The owner requested Go implementation, then asked that proposals match PA#3 and that a separate change be created. Created `implement-pa3-go-prototype` with proposal, design, eight capability deltas and the sole task checklist in [issue #38](https://github.com/JakeConal/leetkob/issues/38). All 37 implementation tasks remain unchecked.
- Changed: the first API/dependency draft included registration, rotating JWT sessions and broader account administration. Revised it to the assessment-account sign-in, authoring, judging, checking and private TA review needed for PA#3; removed the JWT dependency. Concrete dependency/API approval remains pending.
- Changed: reconciled the now-published eight-criterion rubric and advanced public deployment/private review into PA#3 scope. Distinguished valid first-run scorer exit 1 from invalid-input exit 2 without changing the later eval thresholds.
- Rejected: treating the full remaining-product roadmap as this checkpoint, inventing an overall PA#3 eval pass requirement, exposing unconfirmed findings to students, or interpreting a request for a new change as approval for dependencies, paid calls or hosting.
- Preserved the existing umbrella proposal/specs, PA#2 implementation/contracts/tests, official hash manifest and unrelated staged `.opencode` files. No production implementation was edited. Planning validation is separate from evidence that the planned system works.

### 2026-11-23 — PA#3 contract approval and task 1.1

- Tools: OpenCode (GLM-5.3) with the OpenSpec CLI, the in-app browser for the Classroom assignment re-read, GitHub API reads for issue #38, and local repository inspection. No model-provider request or deployment was made.
- Re-read the live PA#3 Classroom assignment, re-audited the change against every requirement, and confirmed the plan covers the assignment without conflict.
- The owner approved the PA#3 dependency inventory and API draft, authorized implementation to start, deferred the paid model run, and asked for free hosting research. Recorded approval in both documents and here as D13.
- Published the approved contract subset before any route exists: OpenAPI 3.1 `pa3-openapi.json`, versioned `job-envelope.v1` and `checker-prediction.v1` schemas, and the `pa3-problems.json` fixtures pinning every RFC 9457 body including the closed-window and stale-revision conflict extensions.
- Added `apps/backend` (task 1.2): Go 1.27.4 module `github.com/JakeConal/leetkob/backend` with `cmd/api`, `cmd/worker` and `cmd/checker` on injected domain ports, bounded Argon2id hashing, the unavailable problem response, and startup/health/shutdown documentation with module pins and transitive licenses. Verified by build/vet/test-race and a live three-command smoke including clean SIGTERM exits.
- Extended the root checks to Go (task 1.4): `lint` also runs `go vet`, `format:check` also runs `gofmt -l`, `typecheck` also compiles the module, `test` also runs `go test -race`, and `security` runs pinned `govulncheck v1.10.0` via `scripts/go-check.mjs`, which fails rather than skips when the pinned toolchain is absent. Demonstrated defect detection with a deliberate failing Go test (root Test exit 1, restored exit 0) and a deliberately unformatted file (root Format exit 1, restored exit 0); neither fault was committed. Added the rubric's concrete never-list (fabricated evidence, unisolated parsing with live keys, weakened sandbox controls) to `AGENTS.md`.
- Added the Go compatibility composition for the PA#2 publication contract (task 1.3): `apps/backend/internal/compatpub` mirrors the HMAC bearer boundary, publication service order, exact-response idempotency scope, per-lab serialization and every RFC 9457 body. The seeded/fixed-clock scenarios in `docs/contract/fixtures/publication-scenarios.json` now run unchanged against both the NestJS app (`apps/api/test/contract/publication-scenarios.contract.test.ts`) and the Go composition (race-enabled), covering authentication, input limits, ownership concealment, the deadline gate, problem bodies/media types, replay and concurrent publication. The original PA#2 suites remain untouched and green (45 API tests).
- Captured the reproducible NestJS/Go comparison (task 1.5): `npm run compare:backends` drives the identical 4,680-request seeded workload against both compositions and writes `docs/evidence/backend-comparison.json` and `backend-comparison.md` from one run. Measured zero errors on both sides, Go compatpub at ~2.7x throughput (2,860 vs 1,047 req/s), p50 0.343 vs 0.849 ms, startup 21.5 vs 175 ms, warm-cache build 641 vs 2,058 ms, with revisions, hardware, warmup/run counts and caveats recorded. Review acceptance stays pending and no default backend switch was made: `apps/api` remains the assessed PA#2 application.
- Committed the previously uncommitted November 20 planning artifacts on `codex/implement-pa3-go-prototype`, preserving the unrelated staged `.opencode` files.

### 2026-11-23 — Hard-rule analyzer (task 5.1)

- Tool: Cursor Grok, local repository only. No model-provider request, no GitHub issue edit, no push.
- Added a native libclang analyzer (`apps/backend/native/cpp-analyzer/analyzer.c`) and a Go consumer that reads its versioned JSON. Go does not use CGO. Ban lists are constraints that start with `Do not use` or `Do not call`; banned names are the backtick tokens, matched to resolved declarations.
- Changed: libc++ reports `std::sort` inside the inline namespace `std::__1`. The qualified name is normalized before comparison so the ban written as `std::sort` still matches, while a function the student declares stays user-declared and does not.
- Rejected: searching the submission text for a banned spelling. A comment-or-string mention, a student `sort`, and `<algorithm>` included only for `std::swap` are not violations. Alias, using-declaration, macro, and wrapper uses cite the original submitted line.
- Unsupported analysis (bad document, diagnostics, unresolved name with no resolved use) is `unsure`, not a violation.
- The new package tests passed with `go test -race -count=1 ./internal/checker/hardrule/ ./internal/checker/schema/`. The native test compiled and ran the analyzer with the local Apple clang and Command Line Tools libclang; it was not skipped. A temporary fault that treated a comment as a reference failed `TestCompliantTrapsAreNotViolations/banned_name_only_in_a_comment_or_string` and was not committed.

### D14 — Changed: adopted the merged PA#3 implementation and ported only unique evidence

- **Initial state:** two parallel PA#3 implementations existed. Our branch `codex/implement-pa3-go-prototype` held 17 unmerged commits (PostgreSQL store, sessions, labs, submissions, worker, sandbox controller, analyzer wrapper, model adapter, review queue, checker CLI); `main` received a composited implementation from Toan Huỳnh through PRs #41/#42 with a different architecture (`db/001_init.sql`, `internal/{access,submit,judge,checking,flow,labs}`, `internal/checker/hardrule` + `native/cpp-analyzer`, live `/live/**` UI). PR #39 had merged only the shared foundation (contract, compatpub, comparison evidence, Go checks).
- **Changed:** adopted `main` as the source of truth instead of merging the duplicate branch (10 direct file conflicts plus duplicate schemas, migrations, sandbox controllers, review packages, workers and web routes). Ported only what `main` lacked: the labelled team cases with Accepted validation and the official byte-integrity reconciliation (task 7.3), a real checker CLI wired around `hardrule` with the missing Dockerfile and a network-none image that matches the labels (tasks 7.1/7.2 gap), and the memory-containment fallback for the flaky CI assertion (exit 137 when Docker's OOM flag races).
- **Rejected:** reconciling two complete implementations days before the deadline, and silently reticking or unticking issue #38 tasks without a mapping.
- **Why:** the rubric's "checker command with a Dockerfile and MODEL_OFF=1" was genuinely missing on `main`, while everything else the team needs is already there and green.
- **Evidence:** the checker image run on `evaluation/team-cases` matched all 10 independent labels with 0 mismatches under `--network none`; the official pinned tree verified 128/128 byte-identical files; the port branch is `codex/port-pa3-unique-evidence`.

### 2026-11-24 — Adopted main, ported unique evidence, first real evaluation

- Tools: local repository inspection, GitHub API, the user's authorized NVIDIA NIM key supplied outside the repository, the pinned official dataset (fetched, verified, never vendored). No deployment was provisioned (owner decision) and no credential was written to the repository, logs or evidence.
- Changed: adopted the merged PA#3 implementation (D14) and ported the missing pieces — a real checker CLI with digest-pinned image around the existing `hardrule` analyzer (tasks 7.1/7.2), labelled team cases with Accepted validation and the official byte-integrity reconciliation (7.3), a repeatable `cmd/provision` and `deploy/` artifacts verified by local staging (8.2), and the `pa3` tag attack gate in CI (8.1).
- Changed: ran the authorized first dev-set evaluation with `minimaxai/minimax-m3.1-flash` through the checker image: 102 predictions, 0 wrongful deductions, 78.4 % recall, 3.9 % unsure, unmodified scorer exit 1. Committed predictions, scorer report and a call log with provider request IDs, tokens, elapsed time and cost; documented the 80-submission arithmetic at the free-tier price (7.4/7.5). A rate-limit retry and a missing CA bundle were found during the run and fixed before the committed run.
- Rejected: inventing a deployment URL or tag-run evidence after the owner chose to skip hosting; counting the online worker's offline semantic results as real model calls; committing any part of the API key.
- Evidence: `evaluation/dev-run/`, `docs/evidence/pa3-walkthrough.md`, `deploy/README.md`, CI run 45306355723, and PR #44.

### 2026-11-27 — Local shell launcher and synthetic E2E seed on current main

- Changed: added `scripts/run-local.sh` to run the current Go API, judging/checking worker and Next.js UI locally. A separate `leetkob-local-v2` Docker Compose project supplies PostgreSQL; the existing isolation controller supplies C++ judging and a new fixed analyzer entry point. The API can bind explicitly to loopback with `API_HOST`. No runtime dependency or API contract changed.
- Changed: added five synthetic accounts, two classes, eight labs and five real submissions, plus repeatable HTTP smoke checks. The database password is generated privately and persisted alongside the seed manifest; existing synthetic volumes adopt it without deleting records. Private generated passwords, state, logs and reports stay in the ignored `.local-dev/v2/` directory. Re-seeding preserved all lab/submission IDs. The v2 project and state stay separate from the earlier prototype schema.
- Rejected: merging the older duplicate implementation over main, parsing submitted C++ on the host, forwarding production environment variables, or counting offline semantic `unsure` findings as model evaluation evidence. Model requests remain off; current main's concealed review-access responses and fixture judge/hard-rule labels are preserved.
- Evidence: local `npm ci --ignore-scripts`, lint, formatting, type checking, production build and security checks passed. The full `npm test` passed with `POSTGRES_URL` pointing to the dedicated synthetic local PostgreSQL instance; the machine's default port belongs to another application. A real `--smoke` run passed AC/hard-rule pass, forbidden-sort violation, injection-comment violation, WA, CE, student result privacy, class/role isolation, expired deadline, empty queue and logout checks. These are local HTTP checks, not browser, deployment or paid-provider evidence.
