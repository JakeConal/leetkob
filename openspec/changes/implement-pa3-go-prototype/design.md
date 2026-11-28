# Design

## Context

See [proposal.md](proposal.md) for the milestone boundary. Source reads on November 20 are recorded in [the acceptance matrix](../../../docs/requirements/acceptance-matrix.md#pa3-published-rubric-and-implementation-deltas). PA#3 is 100 points, 7% of the course, due November 29 at 22:00 Asia/Saigon.

Observed checkout baseline is `ee31dcb` on `codex/pin-evaluation-sources`. The original staged `.opencode` files are unrelated user work and must remain untouched. No implementation or dependency installation is claimed by these planning artifacts.

- `apps/api` is a NestJS/Fastify PA#2 publication implementation with in-memory repositories, signed fixture claims and a 1 KiB global body limit.
- `apps/web` uses the checked-in theme and synthetic review rows; only health is live.
- `evaluation/official-source.json` pins 128 original files at `552c811db2d35a23d641bc4fdffc1ac120be1a28`; scorer/label hashes match the PA#3 rubric prefixes. The source verifier/stager is implemented, but no checker is.
- There is no Go module, durable database adapter, judge, worker, model adapter, live queue or deployment configuration. The existing workflow listens to `pa2`, not `pa3`.
- Existing `complete-leetkob-product-workflow` approves the direction of a Go migration but not new dependencies/contracts or default-backend cutover. Its issue #7 is a roadmap, not proof that its tasks are complete.

## Goals / Non-Goals

**Goals:** Deliver the PA#3 acceptance scenarios in eight capability deltas; reuse the Go API/worker/checker architecture and existing frontend; retain domain/storage/transport boundaries; make every rubric claim traceable to current evidence; keep TA findings private.

**Non-Goals:** Implement the full umbrella roadmap before PA#3, replace Next.js, add another programming language, or introduce production grades/appeals. PA#3 requires a first real eval, not proof that the later PA#4 quality thresholds pass. No paid call, external deployment or coursework submission is implicitly approved by creating this change.

## Decisions

### 1. Separate milestone ownership and preserve compatibility

This change owns the PA#3 implementation and its task issue. The existing umbrella change remains the broader product roadmap. Its task IDs are reference mappings only; marking a milestone task complete must not close a larger umbrella task with additional acceptance criteria.

Use `apps/backend/cmd/api`, `cmd/worker` and `cmd/checker`, sharing packages under `internal`. Keep the existing PA#2 compatibility app and all its assertions intact. Go production routes use `/v2` and cannot register the old publication handler against production storage.

Before a default backend switch, reuse the umbrella migration prerequisites: equivalent publication fixtures/clock, HTTP parity, complete Go/Node checks and a reviewed reproducible comparison. New Go production work can run on a separate explicit development port while those gates are pending; it does not silently replace the existing default. Review the final cutover in the actual implementation PR.

Alternative: replacing the old handler in place would mutate already-assessed authentication/replay behavior and create a route around production safeguards.

Before archiving either change, rebase its overlapping capability deltas against current main specs, preserve any remaining later requirements, and reconcile issue #7 only against delivered evidence. Do not archive both overlapping ADDED deltas unchanged.

### 2. One PostgreSQL store and a small assessment identity boundary

Use PostgreSQL for users, session-token hashes, current memberships, immutable lab input revisions, submissions, counted selection, jobs, findings, TA decisions, replay records and budget reservations. Domain ports own transactions; neither HTTP handlers nor model adapters become the business-policy owner.

The proposed concrete runtime inventory is [pa3-dependencies.md](../../../docs/requirements/pa3-dependencies.md). Owner approval remains pending. Proposed additions are Go 1.27.4, pgx v5.11.0, x/crypto v0.59.0 Argon2id, PostgreSQL 18 and isolated LLVM/Clang/GCC image tooling. Pin exact module sums, image digests and installed native-package versions during build qualification; no JWT/provider/telemetry SDK is needed.

Opaque eight-hour sessions use random tokens stored as hashes and current database authorization on every request. HttpOnly same-origin cookies plus CSRF/origin enforcement serve the browser and API clients. Account provisioning creates only the five synthetic assessment accounts, two classes and memberships; the TA authors the actual marked lab through the product.

This is an explicit PA#3 staging decision, not completion of the umbrella registration/refresh/JWT requirements. Preserve identity ports so later account lifecycle work does not affect domain rules.

Alternative: embedded long-lived role/class claims would ignore removed memberships. Full self-service registration/federation would add unassessed work before the core flow exists.

### 3. Approve a language-neutral PA3 API before adding routes

The review draft is [pa3-api.md](../../../docs/contract/pa3-api.md), revised at the owner's request to exclude later features. Owner contract approval is still pending; requesting a separate change did not approve the draft.

Publish the approved subset as OpenAPI 3.1 and generate narrow frontend types with a deterministic drift check before wiring live views. Route families are sessions/classes, TA lab creation, submissions/results and TA reviews/decisions. Mutation contracts define current authorization, bounded bodies, CSRF, idempotency fingerprints/replays and safe RFC 9457 errors. UUIDs and UTC instants cross the API; deductions and costs use fixed-precision integer storage.

Tests/constraints enter through a bounded structured lab-creation request. ZIP ingestion and revision/editing routes are later additions with their own lifecycle contracts, not unvalidated alternate ingress paths. No lab seed script or manual database step may be needed for the criterion-1 walkthrough.

Alternative: exposing all planned `/v2` families now would make PA#3 depend on unassessed account/grade/appeal features and more contract review.

### 4. Accept and recover work transactionally

Ingress authenticates and obtains a bounded admission lease before consuming source. Stamp receipt when the complete bounded body arrives. Reject completion at/after the deadline and expired leases. Commit source, submission, counted selection, judge intent and replay result atomically; no acknowledgement precedes durable commit.

Completed same-key/content retries replay after the deadline once current ownership is checked. Different content conflicts. Durable server order resolves equal receipt times. Later WA remains counted over earlier AC; out-of-order workers cannot restore old findings.

Workers lease database jobs with an increasing attempt fence and bounded infrastructure retries. Commit judge result, eligibility and checker intent together. Unique revision keys prevent duplicate stored effects; stale fences cannot commit. Visible failed infrastructure work blocks readiness rather than becoming a student verdict.

Alternative: an in-process request-handler call fails the separate-worker requirement and the required stop/submit/restart demonstration. An external broker would add an unnecessary cross-store acceptance boundary.

### 5. Qualify execution and parsing isolation before enabling source execution

Use a dedicated Linux execution adapter. Submitted compilation/execution receives a fixed compiler image and copied task inputs in disposable storage, with no host-directory binds or Docker socket. It must enforce all nine rubric controls; configure non-root, no network, dropped capabilities/no new privileges, memory and equal swap ceilings, PID limits, bounded output and a host-enforced deadline that removes the entire container.

The trusted controller may access Docker; no such access enters the submitted process. Provision compiler/runtime files in the image. Untrusted preprocessing and syntax analysis use the same secret-free isolation principle: clearing environment alone does not prevent `#include` of a host credential file.

Qualify the actual configuration with repository attack submissions and observable negative controls. A configured flag alone is not proof. Keep attacks contained to disposable test resources.

| Rubric minimum        | Attack and independent observation                                                                                                                                                 |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| No network            | Attempt DNS/outbound access to an otherwise reachable controlled fixture; isolation denies it.                                                                                     |
| CPU                   | Infinite compute loop reaches configured CPU enforcement.                                                                                                                          |
| Wall time             | Sleeping/blocking submission reaches deadline; inspect that the container is gone, not merely its client process.                                                                  |
| Memory including swap | Allocate beyond limit and read written values back; inspect memory/swap configuration and containment so optimization cannot remove the allocation.                                |
| Processes             | Bounded fork/thread attempts reach the PID ceiling without affecting other runs.                                                                                                   |
| Output                | Flood stdout/stderr beyond the configured cap; collection stays bounded and the run ends.                                                                                          |
| Non-root              | UID/privileged-operation probe proves an unprivileged user.                                                                                                                        |
| No host mounts/socket | Inspect actual mount configuration and probe synthetic host canary/socket; neither is accessible.                                                                                  |
| No secrets            | Synthetic canaries placed only in trusted fixture configuration remain absent from environment, accessible files and compiler inclusion. Never use a live credential as the probe. |

Compile failure/timeout maps to CE; CPU/wall timeout to TLE; output overflow to RE; distinguish MLE where reliable or document memory RE. Compare output tokens. Infrastructure failure remains a separate operational state. Every control must run on the `pa3` tag's real Linux CI image, including compilation isolation.

Alternative: default containers without measured controls, host compilation, or bind-mounting a working directory cannot satisfy the minimums.

### 6. Resolve hard rules using C++ declarations and preserve privacy

Build a small libclang-based analyzer in the execution image; Go consumes a bounded versioned result, without CGO. Resolve actual declarations/references and original locations for backticked ban names. Cover student-defined namespace functions, comments/strings, harmless includes, namespace aliases, using, macros and wrappers with tests. Generic rules receive constraint text rather than dev-problem IDs.

Online worker analysis runs in the credential-free execution boundary before trusted model dispatch. The offline checker image includes the analyzer locally and contains no provider key, labels or trusted broker configuration. It requires no nested Docker socket. Unsupported analysis produces unsure, never an invented violation; the required dev ban-list coverage remains an acceptance obligation rather than being waived by that fallback.

Use lexer-aware comment removal and known account-name/ID anonymization across outbound fields, preserving newlines/original-line mapping. Ambiguous unsafe transformations become unsure without transmission. Keep remaining code explicitly untrusted in the prompt. Send only statement, constraint IDs/text and sanitized source, not tests or account metadata; the model never receives authority to set a deduction.

Alternative: regex ban searches misclassify the rubric traps. Running the compiler/parser beside model credentials would expose include-visible secrets even if its environment were filtered.

### 7. Guard semantic calls and account for spend before dispatch

Use standard-library HTTP behind an injected model port. Real calls happen only on the deployment and separately authorized dev evaluation. Require strict result fields, all requested constraint IDs, unique results, enum verdicts and original-line evidence. Unknown score/deduction fields or fabricated evidence fail validation. One schema repair at most; timeout/provider/schema/evidence failures become distinct unsure causes.

Reserve a conservative priced maximum in one transaction against the lab's USD 0.25 cap before every call/retry. Include rechecks and concurrent requests. Unknown pricing refuses dispatch; unknown usage retains the reservation. Set a bounded output limit and record actual settled usage when known.

Retain sanitized prompt/call provenance in the restricted committed dev-run evidence: provider request ID, model/version, prompt hash, tokens, elapsed time, cost and source/data revisions. General logs and student routes exclude model contents and identities. Hard-rule-decided constraints generate no model call, visible in the call evidence.

Alternative: free-text storage, unbounded calls or charging zero after a timeout fail both rubric and spending policy.

### 8. Deliver a private queue with no production publication path

Store findings with submission/input revisions, source kind, original evidence and TA-configured deduction. Required obligations are every violation/unsure and a stable random ceil(10%) pass sample. Queue reads do not redraw samples. Explicit TA decisions require current revision and valid original evidence for violation; append audit actor/time without overwriting checker provenance.

The student DTO is a positive allowlist of judge state/verdict/test timing only. It never serializes internal findings. Cite the existing pinned FAQ-08 for this pre-publication behavior. Live pages use current user/class query scopes, no shared protected-data caching, and clear data on logout.

No Go production Publish or grade endpoint is enabled for PA#3. The historical publication process stays isolated with fixture-only stores. If scope later adds publication, it requires its own reviewed contract and tests refusing all pending reviews/processing; a UI-disabled button alone is insufficient.

Alternative: exposing model findings in student responses would trigger the rubric's zero scores for criteria 4, 5 and 6.

### 9. Make the marking commands reproducible without gaming the eval

Build `checker --data <dir> --predictions <file>` from the same checker core as the worker. Generic discovery reads statements/constraints/source only. Stage prediction inputs with the existing allowlist; trusted scoring separately accesses official labels/scorer. Verify official hashes before/after scoring and preserve byte-identical dev inputs in the submitted repository as required by criterion 6, reconciling the existing provenance policy explicitly.

MODEL_OFF=1 is selected before creating any provider client or loading credentials. The image contains the analyzer and writes unsure for all semantic constraints without any network. Exercise it with the marker's network-none, memory/PID limits and ten-minute kill boundary. An unmodified scorer exit 0 or 1 is valid PA#3 output; exit 2 is an invalid run. This does not redefine the later PA#4 passing thresholds or the existing evaluation-gate policy.

Add at least five separate labelled/tagged team cases, including trap/injection. Validate their C++17 Accepted status and establish labels independently of predictions. Do not touch published solutions, labels or eval.mjs, use labels to generate predictions, or hard-code dev answers.

The first authorized real dev run commits predictions, actual scorer report and call provenance. Report actual wrongful deductions, recall and unsure, even on threshold failure. Derive the 80-submission cost from observed call batching/token use and documented model prices, with arithmetic in README and AI-LOG.

### 10. Keep implementation and release evidence distinct

Implement focused test-first slices. Hard-rule/trap and invalid-model/timeout tests land before or with their code. Commit each real part as it is built; never split finished history afterward to simulate the rubric requirement. Record real changed/rejected decisions in AI-LOG.

Root commands retain required result names and cover all Go/Node workspaces, binaries, formatting, vet, tests/builds, supported races and vulnerability scans. Tag CI gains `pa3` alongside `pa2` and actual sandbox tests without provider keys. Ordinary checks run with dependencies prepared and network cut within the marker's 15-minute bound. Request approval for CI/protection changes where required; do not bypass protected main.

Public deployment is a PA#3 release requirement, not phase-8-only work. Prepare repeatable containers/runbook first; obtain concrete hosting/spend authorization before provisioning. Keep the approved service up from deadline until PA#4 marks. PA3.md then records actual URL, five synthetic accounts, tag CI, checker commands, committed run and <=5-minute recording. Self-assessment must reference current evidence for all eight rows; no placeholder URL/run, invented cost or assumed score.

## Scenario-to-evidence plan

The task issue owns the executable checklist. Commands below describe checks to add; they are not claims that missing Go packages already exist.

| Scenarios                         | Planned entry point and falsification                                                                                                                                           |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| PA3-ID-01–06                      | Real session/class HTTP contracts with durable adapter; wrong-role/cross-class/revoked/CSRF attempts must fail without mutation.                                                |
| PA3-LAB-01–04                     | TA authoring HTTP and browser flow using supplied tests/constraints; database-empty-lab setup proves no lab seed dependency; invalid creation rolls back.                       |
| PA3-JUDGE-01–04,08                | API plus separate workers against disposable PostgreSQL; stop/restart, duplicate claims, expired fences and later-WA traces prove durable uniqueness/current counted selection. |
| PA3-JUDGE-05–06                   | Actual container attack suite and inspected state on tag CI; naive/disabled-control negative checks demonstrate tests can detect absent enforcement.                            |
| PA3-JUDGE-07, PA3-UI-03           | Integrated real judge with compliant/violation/injection marking submissions; no fake judge for the walkthrough.                                                                |
| PA3-CHECK-01–02                   | Generic syntax analyzer fixtures plus unchanged dev ban-list model-off predictions; zero false violation and all ban-list pairs decided.                                        |
| PA3-CHECK-03–06                   | Fake-provider schema/evidence/timeout/privacy/budget tests with network unavailable; assert dispatch count and conservation under concurrent reservations.                      |
| PA3-CHECK-07, PA3-EVAL-04–05      | Explicitly authorized real dev run, committed predictions/call IDs and independent unmodified rescore.                                                                          |
| PA3-EVAL-01–03                    | Built image on unseen fixture and published dev set under marker-equivalent restrictions; inspect output validity, zero connections and source hashes.                          |
| PA3-REVIEW-01–06, PA3-UI-01–02,04 | Assigned TA/student HTTP/browser negative projections, explicit stale decision conflict, stable sampling and narrow/keyboard UI review.                                         |
| PA3-GATE-01–03                    | Root checks, deliberate detected failures, real focused history and actual pa3 tag-CI results; no rebuilt history.                                                              |
| PA3-GATE-04–05                    | Approved deployed synthetic-account walkthrough, <=5-minute fallback recording and clean extracted-tag archive verification.                                                    |

## Risks / Trade-offs

- **Five-day milestone and Go migration cost** -> Implement this bounded flow, preserve the compatibility route, and measure/review default cutover separately; do not claim unperformed umbrella work.
- **Native analyzer/runtime differences** -> Pin the qualified image/packages and run traps/attacks on that exact artifact; no production execution until all controls are proven.
- **Model/provider quality or cost** -> Keep selection configurable behind the port, test limits offline, and retain actual first-run results instead of tuning labels.
- **Public test accounts can consume budget** -> Use synthetic data, enforce per-lab and deployment-level caps/rate limits, and agree hosting duration/cost before deployment.
- **Source ownership overlap** -> Before archive, reconcile capability deltas/tasks with the umbrella; PA#3 completion does not automatically complete later APIs or gate thresholds.
- **Approval dependencies** -> New runtime inventory/contracts remain pending; paid evaluation and hosting are distinct release decisions. Prepare concrete review artifacts before requesting each action.

## Migration Plan

1. Review this change and the concrete dependency/API drafts; resolve approvals and preserve unrelated staged user files. Use a dedicated feature branch without resetting the existing work.
2. Add Go scaffolding and equivalent publication fixtures, then root/CI coverage and reviewed comparison before any default switch. Keep the historical artifact available for compatibility rollback.
3. Add only additive PostgreSQL initialization and production API/worker/checker composition. Exercise failure rollback/recovery with disposable data; never promote demo rows as real student records.
4. Qualify sandbox/analyzer images, guarded checker and real user flow. Finish offline marking checks before requesting a paid run or deployment.
5. Perform the separately authorized real eval and deployment, verify the live walkthrough, then create evidence-linked `pa3` tag/CI/archive. Never submit coursework automatically.
6. Keep schema compatible during rollback; stop/fence work during unsafe upgrades and recover from durable jobs. Destructive database restore retains explicit approval requirements.

## Open Questions

- Which configured model/version and verified price schedule produce useful first-run metrics within the USD 0.25 policy? Decide before the bounded paid-run authorization, without changing the schema or checker interface.
- Which approved hosting target and explicit spending/duration ceiling will keep the qualified Linux execution service reachable through PA#4 marking? Select before provisioning; prepare the same deployable artifact independently.
