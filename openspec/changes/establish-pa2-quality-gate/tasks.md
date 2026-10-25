# Tasks

## 1. Repository harness and rules

- [x] 1.1 Add Node 22+ ESM TypeScript project metadata, lockfile, strict compiler configuration, ESLint, Prettier, and the `lint`, `format:check`, `typecheck`, and `test` scripts; verify `npm ci` succeeds from the lockfile.
- [x] 1.2 Add `.gitignore` and a value-free `.env.example` covering every supported variable; verify ignored local environment files do not appear in `git status` and the example contains no assigned secret value.
- [x] 1.3 Add concise `AGENTS.md` sections for Stack, Commands, Conventions, Never, Allowed, Ask first, and Never permissions; verify all rubric phrases are present and two product-specific rules cite concrete `AI-LOG.md` entries.
- [x] 1.4 Add `README.md` setup and command documentation, PA#2 limitations, authentication boundary, model provider policy, a numeric per-lab cap, and the no-real-model-test rule; verify every documented setup/check command runs as written.

## 2. Publication domain and authentication boundary

- [x] 2.1 Implement strict lab, publication, identity, problem-details, repository, clock, and idempotency types plus isolated in-memory adapters; verify unit tests cover state reads/writes and stored response replay.
- [x] 2.2 Implement minimal HMAC bearer-token issue/verify helpers with constant-time signature comparison and strict claim validation; verify tests reject absent, malformed, tampered, expired, and structurally invalid tokens.
- [x] 2.3 Implement the publication service with role, class ownership, pending-review, atomic transition, already-published, and exact-response idempotency rules; verify domain tests cover every branch without network or model calls.

## 3. HTTP contract and biting tests

- [x] 3.1 Implement the Node HTTP adapter and production entry point for `POST /labs/{labId}/publish`, including bounded body handling, header/UUID validation, safe JSON serialization, and `application/problem+json`; verify an ephemeral server returns the documented content types.
- [x] 3.2 Write `docs/contract/publish-lab.md` as a standalone client contract covering caller role and ownership, authentication, request/response field rules, state changes, idempotency, every RFC 9457 problem type/status, and examples; verify a reviewer needs no implementation source to construct requests.
- [x] 3.3 Add HTTP contract tests for success, every error type, another TA's class, pending reviews, same-key success and rejection replay, different-key conflict, atomicity, and response privacy; verify `npm test` passes and no test performs a real external request or model call.

## 4. Local and CI quality gate

- [x] 4.1 Add a high-confidence tree/history credential scanner and dependency audit script without embedding example credential strings; verify it passes on the repository and fails on a temporary synthetic fixture.
- [x] 4.2 Add a GitHub Actions workflow for pull requests, `main`, and `pa2` tags with independent Lint, Format, Typecheck, Test, and Security jobs that run the documented commands after `npm ci`; verify the YAML references the same package scripts as the README.
- [x] 4.3 Run formatting and all local checks, repair every failure without weakening tests or contract requirements, and verify `npm run lint`, `npm run format:check`, `npm run typecheck`, `npm test`, and the security command are green.
- [x] 4.4 Complete `AI-LOG.md` with actual tool use and specific Changed/Rejected decisions from groups 1–4; verify each cited decision maps to a committed rule, contract, test, or implementation choice.

## 5. Green baseline and protected main

- [x] 5.1 Commit the complete green baseline and push it to `main` before protection; verify the GitHub Actions run finishes green for every job.
- [x] 5.2 Read the exact observed check contexts, enable public `main` branch protection with administrator enforcement, required pull-request flow, and the four rubric checks, and verify the protection API reports the intended settings.
- [x] 5.3 Attempt a direct non-destructive push probe only through GitHub's protection-aware flow or API and verify later changes must use pull requests without adding a commit to `main`.

## 6. Modern workspace migration

- [x] 6.1 Convert the repository to npm workspaces with `apps/api` and `apps/web`, add only the approved NestJS/Fastify, Next.js, React, and TanStack Query dependencies, and verify a fresh root `npm ci` installs both applications.
- [x] 6.2 Move the existing domain, authentication, and in-memory adapters into `apps/api` without coupling domain code to NestJS; verify the original unit coverage remains green.
- [x] 6.3 Replace the raw HTTP adapter with a NestJS module, controller, RFC 9457 filter, and Fastify bootstrap for `POST /labs/{labId}/publish` plus `GET /health`; verify the unchanged publication contract over an ephemeral Fastify listener.
- [x] 6.4 Implement the Next.js App Router dashboard from the authoritative `docs/design/theme.md`, use TanStack Query for health loading/error/retry state, and verify desktop and narrow responsive layouts in a real browser.
- [x] 6.5 Update the root checks, CI, `AGENTS.md`, `.env.example`, README, and AI log for the workspace architecture; verify every documented command runs as written and no dashboard state exposes protected data.
- [x] 6.6 Commit the migration in coherent increments, open a protected pull request, wait for every required check, and merge only after the implementation and visual verification are green.

## 6A. Product-owner FAQ-12 alignment

- [x] 6.7 Amend the proposal, design, and delta specs with the product-owner rule that whole-lab publication requires the submission deadline to have passed as well as a fully decided review queue; cite FAQ-12's immutable source commit.
- [x] 6.8 Update the standalone contract and add domain and HTTP contract tests for pre-deadline rejection, exact-deadline success, check ordering, state safety, problem details, and exact-response replay before changing the implementation.
- [x] 6.9 Add the server-clock deadline gate to the framework-independent publication service and lab model, including the documented RFC 9457 problem type; verify all unchanged and new tests pass.
- [x] 6.10 Update the Swiss Minimalist dashboard, README, and AI log to expose both FAQ-12 preconditions without protected data; verify desktop and narrow layouts plus all repository checks.

## 7. Demonstrate that the gate bites

- [x] 7.1 Create a branch containing one deliberate implementation-only contract regression while leaving the contract and tests unchanged, open a pull request, and verify the Test job is red with the expected contract-test failure.
- [x] 7.2 Verify GitHub reports the red pull request as blocked by required checks and record the pull request, failing run, and deliberate-break commit URLs for `PA2.md`.
- [x] 7.3 Repair only the implementation, commit the fix, and verify the same pull request turns fully green with the contract and tests still unchanged.
- [x] 7.4 Merge the repaired pull request through the protected flow and verify the merge commit is on `main` and no direct post-protection commit was introduced.

## 8. Evidence and self-assessment

- [x] 8.1 Create `PA2.md` with public links to the repository, protected-branch settings evidence, blocked pull request, failing deliberate-break run, green repair commit/run, and placeholder for the tag run; verify every non-placeholder link resolves publicly.
- [x] 8.2 Create `SELF_ASSESSMENT_REPORT.md` with one rubric row per criterion, conservative claimed points, and concrete file/commit/PR/CI evidence plus a short not-managed section; verify the sum matches the intended archive filename total.
- [x] 8.3 Update `AI-LOG.md` with the deliberate failure, repair, branch-protection decisions, and any rejected agent suggestions; verify entries are factual and reference immutable evidence.
- [x] 8.4 Deliver groups 8.1–8.3 through a second protected pull request and verify all required checks are green before merge.

## 9. Final verification, grading, tag, and archive

- [ ] 9.1 Run OpenSpec verification against `establish-pa2-quality-gate` and resolve every critical issue, warning, and actionable suggestion; verify a second report is clean.
- [ ] 9.2 Run all five local checks from a fresh `npm ci`, audit the reachable Git history for credentials, and verify the working tree is clean and synchronized with protected `main`.
- [ ] 9.3 Have an independent grading agent score the repository against the full PA#2 rubric, fix every substantiated deduction through protected pull-request flow, and repeat grading until the evidence supports the final claimed score.
- [ ] 9.4 Create and push annotated tag `pa2` at the final merge, wait for the tag CI run, add its immutable URL to `PA2.md` through protected flow if needed, and verify the remote tag resolves to the intended commit.
- [ ] 9.5 Create `23120124_<self-assessed-total>.zip` outside the repository with the entire repository including `.git`; verify extraction succeeds and required root files plus `.git` are present.

## Workflow follow-up

- Archive `establish-pa2-quality-gate` after all implementation tasks and final evidence are complete.
- Verify the archived OpenSpec capability specs match the tagged implementation.
- The user submits the verified zip to Google Classroom; this plan does not submit academic work on the user's behalf.
