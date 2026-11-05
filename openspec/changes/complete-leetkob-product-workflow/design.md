# Design

## Context

See `proposal.md` for motivation and the capability deltas for behavioral acceptance. This is an umbrella change delivered through sequential, reviewable phases; it is not one deployment or one pull request.

Observed in the checkout on 2026-11-05:

- `apps/api/src/domain/publication-service.ts` injects repositories and a clock, but serializes only within one process and saves the lab before separately saving the replay response. Durable multi-process publication therefore needs a unit-of-work boundary, not just replacement map adapters.
- `apps/api/src/auth/token.ts` verifies a custom two-segment HMAC claim with embedded roles/classes. This is the documented PA#2 boundary, not a complete account/session system.
- `apps/api/src/app-factory.ts` defaults to empty in-memory stores, a 1 KiB body limit, and GET/POST CORS. Production uploads and routes require explicit per-route limits and a separate production composition root.
- `apps/web/app/page.tsx` and `lib/review-data.ts` contain synthetic review rows; only `components/system-status.tsx` fetches live data. Existing web tests cover health parsing and the two publication preconditions.
- The publication contract and domain/contract suites already cover ownership concealment, deadline checks, exact replay, and safe errors. `.github/workflows/quality.yml` runs the existing root checks, build, and security scan. There are no durable database, execution, checker, evaluation, or deployment modules yet.
- `.gitignore` and `.agents/skills/.openspec-target` had pre-existing working-tree changes. They are outside this planning change.

### Source authority and scope accounting

Authority is class brief, then FAQ, then the upstream team's proposal. The brief was referenced by repository history but its full contents were not available in this session. Requirements below are grounded in the supplied FAQ/proposal and official evaluation README, not a claim to have independently verified every course rubric. Retrieve the exact brief and checkpoint rubrics for release evidence; discrepancies require explicit review rather than silently weakening these specifications.

Sources read:

- [FAQ](https://github.com/tkob-team/LeeTKOB-FAQ/blob/main/FAQ.md), content blob `4522687ccdab0bd5d7f6e3c0060410d7bc05bd44`.
- [Detailed proposal](https://github.com/tkob-team/proposal-AWAD/blob/main/docs/proposal-detail.md), content blob `202864d7dd52cd05172738eadecbfee5f78b516e`.
- [Evaluation README](https://github.com/fithcmus/awad-leetkob-eval/blob/main/README.md), content blob `36388cfb4120d429ae3abe03006c8bdafb200edd`.

The hashes above identify file blobs, not commit refs. Implementation must pin the actual dataset/scorer commit when vendoring or fetching evaluation inputs. Source model names and pricing estimates are not adopted as verified current provider facts.

## Goals / Non-Goals

**Goals:** Keep domain policy independent of HTTP, identity providers, storage, and workers; make version changes and concurrent processing explicit; use one transactional source of business truth; keep model and execution credentials out of untrusted execution; preserve the PA#2 compatibility suite.

**Non-Goals:** A wholesale stack rewrite, mandatory Java/Python services, five independent storage systems, or adopting the upstream team's service split as a course requirement. The seven Could features remain deferred: B3, C9, F9, G5, H1, H2, I9. No extra language, late submissions, per-student extensions, automatic partial test scores, plagiarism detection, chatbot, agent tools, generated tests, or model-chosen deductions. Daily automated backups are deferred as I9; migration backup/restore and recovery rehearsal remain release requirements.

## Decisions

### 1. Retain a TypeScript modular application with independently run workers

Keep NestJS/Fastify and Next.js. Introduce cohesive API modules for identity, classes, labs, submissions, review, grades, and appeals. Share pure value types and schemas through an npm workspace package; keep domain operations free of NestJS and database imports. Add a TypeScript worker application and a checker package with both worker and CLI adapters. Judge execution runs in a dedicated Linux execution boundary.

```text
Next.js views --> NestJS API --> PostgreSQL
                     |          state + jobs + outbox
                     |                  |
                     |           leased worker jobs
                     |                  |
                     |         +--------+---------+
                     |         |                  |
                     |    judge adapter      checker core --> model adapter
                     |         |                  |
                     |    isolated C++       versioned findings
                     |         |                  |
                     +---------+------------------+
                               |
                      TA decisions --> publication --> grades / appeals
```

Use PostgreSQL for relational state, bounded source/test objects, durable jobs, idempotency, and outbox events. At the stated class scale this permits atomic acceptance without coordinating external object storage and a queue broker. Source is limited to 64 KiB. Ingest test ZIPs into bounded, validated test objects; proposed ceilings are 10 MiB compressed, 100 MiB expanded, and 1,000 entries per bundle, configurable downward. These ingestion ceilings are implementation defaults, not FAQ requirements.

Alternatives considered: the upstream Node/Java/Python + RabbitMQ/Redis/MongoDB/Garage deployment creates more operational boundaries than the current repository needs. A database-backed queue and bounded object tables reduce partial-commit failure modes. Storage and job ports allow later replacement if measured workload warrants it.

Proposed runtime additions include a PostgreSQL driver, standards-based JWT verification, a syntax parser/C++ grammar, and telemetry support. Exact versions and licenses are selected against the existing Node baseline before installation, with repository-required dependency approval. This proposal adds no dependencies.

### 2. Model identity and ownership as current database facts

Use email/password accounts with a memory-hard password hash, signed short-lived access JWTs, server-owned session families, and one-time rotating refresh tokens stored as hashes. Proposed durations are 15 minutes access and 7 days refresh. Store browser sessions in Secure, HttpOnly cookies through a same-origin application boundary; keep any service access token out of localStorage. Validate origin/CSRF on cookie-authenticated mutations. Limit login/reset/invitation attempts using durable counters before adding another infrastructure service.

JWTs identify subject/session, not final class authority. Query current role and membership at each protected request and on event delivery. Revocation invalidates sessions and streams. Administrator role grants are audited; registration cannot self-elevate. Seed the first admin through a documented operator action using separately provisioned secrets, never a committed password.

Support invitation enrollment and TA roster entry/CSV. Validate rows and report per-row failures; accepted rows are individually idempotent. A class row lock enforces the 100-student limit even under concurrent enrollment. Freeze the grade roster at the submission deadline; subsequent class membership changes do not rewrite historical lab grade ownership.

Alternatives considered: keeping embedded long-lived class claims would let removed TAs continue accessing data. An external identity provider remains a possible adapter; account behavior and ownership checks stay local and testable. Google sign-in/reset email are included in the enhancement phase with audited linking and single-use reset tokens.

### 3. Use immutable revisions and a current-generation pointer

Core records:

| Group | Records and important uniqueness boundaries |
| --- | --- |
| Identity | User, SessionFamily, RefreshTokenHash, Class, ClassMembership; one membership per user/class |
| Authoring | Lab, LabRosterSnapshot, TestRevision, TestObject, Constraint, ConstraintRevision; no destructive revision overwrite |
| Submission | Submission, SourceObject, CountedSubmission; unique student/lab/key and immutable source digest |
| Processing | Job, JobAttempt, JudgeResult, CheckRun, Finding; results carry submission/test/constraint/checker revisions |
| Review | ReviewGeneration, PassSample, ReviewDecision; one current decision per finding revision with append-only history |
| Grades | Publication, GradeVersion, Deduction, ManualAdjustment, Appeal; one appeal per published deduction |
| Reliability | IdempotencyResponse, ModelBudgetReservation, OutboxEvent, AuditEvent |

Use fixed-precision decimal points for deductions/scores and integer micro-USD for budgets. UTC instants are authoritative; UI renders local time with an explicit timezone. Hash immutable source bytes and retain original line text separately from sanitized checker input.

The lab tracks active test/constraint revisions and a monotonically increasing grading generation. Jobs carry input digests and generation identifiers. Result ingestion validates them; superseded results remain historical and cannot update current views. Do not let late workers set a bare `pendingReviews` counter.

Alternatives considered: mutating rows in place loses evidence provenance and makes stale worker completion indistinguishable from current work. Revision tuples and compare-and-set writes make invalidation explicit.

### 4. Make submission receipt and job acceptance durable together

Record receipt when the full bounded request has arrived at the API. A database transaction locks the lab admission/finalization boundary, validates server receipt time, inserts source/submission, chooses the latest submission by `(receivedAt, acceptanceSequence)`, and inserts its judge job. Only acknowledge after commit. Give source uploads a short request deadline to prevent indefinitely partial requests.

Finalization and publication must wait for in-flight admissions registered before the deadline to commit or abort. At the trusted ingress, register a short admission lease after checking request headers/identity and before consuming the submission body; it is not yet a valid submission. Stamp receipt when the bounded body finishes, reject completion at or after the deadline, and persist a successful submission before releasing the lease. Deadline finalization checks outstanding leases under the same ordering lock, including requests whose bodies are still arriving. Fencing prevents an expired admission from later committing. Rate limits and the short body timeout bound slow-client blocking. This closes the race where publication finishes while an already-received request is waiting to insert. Production traffic must use this ingress path, not a bypassing upload route.

Submission replay checks authenticated ownership and a completed matching fingerprint before applying the new-submission deadline rule. Same key/content returns the original accepted ID after the deadline. Same key/different content returns 409. New invalid requests cannot change counted selection.

Database jobs use leased claims and monotonically increasing attempt fencing. Expired leases are reclaimable. Commit result ingestion and next-stage intent together; unique input/revision keys prevent duplicate effects. Use bounded exponential backoff for infrastructure failures and a visible failed-job state after exhaustion. Model schema retry is separately limited to one; a generic queue retry must never silently reset model budget or retry limits.

Alternatives considered: an external broker would require a transactional outbox plus separate delivery reconciliation from day one. The selected design still uses an outbox for realtime/email but keeps acceptance and work scheduling in one store.

### 5. Isolate compilation and test execution from trusted services

The execution service accepts staged source and test data plus bounded execution policy. Use a dedicated Linux host/service with an isolation backend that enforces namespaces, resource controls, a non-root submitted process, an isolated filesystem, no network, no host-directory bind mounts, and no Docker socket. Provision compiler/runtime files inside the execution image. Copy only task inputs into disposable task storage; no database or model credential crosses this boundary. Compilation receives the same isolation as execution.

The execution adapter owns backend-specific verdict mapping. CPU/wall overflow becomes TLE, compilation timeout CE, output overflow RE, and memory exhaustion MLE where distinguished or documented RE otherwise. Denied process creation is not falsely reported as a distinct mandatory verdict. Aggregate verdict uses CE for compile failure and otherwise the first non-AC test in stable test order; all tests passing is AC. Infrastructure failure remains separate from student verdicts.

Select the isolation backend through a bounded Linux feasibility exercise against the exact spec attack matrix before normal student execution. The architecture and behavioral contract do not depend on its implementation brand. A backend that cannot enforce any mandatory isolation property cannot be enabled for production. Host/runtime selection is an implementation decision; no claim that a default Docker container alone meets the requirement is made.

Alternatives considered: executing source in the API process or giving a worker unrestricted host execution cannot satisfy isolation. Adding Java solely for dispatch is unnecessary; the backend adapter can call a separately provisioned execution service without changing domain policy.

### 6. Keep checking deterministic where possible and fail uncertain cases to TA review

The checker consumes generic statement/constraints/source input and never labels or per-problem switches. Use a C++ syntax tree for ban-list analysis, with bounded resolution of namespaces, using declarations, aliases, macros, and wrappers. Unsupported resolution produces unsure. Semantic constraints go to the configured provider through a small adapter; the repository's intended OpenAI-compatible integration and USD 0.25 cap remain the default, not the upstream proposal's provider choice.

Sanitization removes comments with lexer awareness, removes known enrollment names/IDs from all outbound fields, and anonymizes identifying material while tracking original line mappings. It must preserve relevant code semantics; ambiguous or unsafe transformation produces unsure without transmission. Original evidence text is restored from the submitted source by validated line mapping, never trusted from free-form model text alone. Do not redact arbitrary identifiers just because they resemble English names. Personal names outside known identity data require conservative detection and review; sanitization is tested as a privacy boundary, not claimed perfect through regex.

Use a strict JSON schema with only declared constraint IDs and verdict/reason/evidence fields. Semantic calls can batch constraints for one solution; cost calculations use actual batching rather than the upstream per-pair estimate. One schema-repair retry is allowed. Wrong evidence, persistent invalid output, timeout, provider outage, budget exhaustion, and preparation failure map to distinct unsure causes. Store raw provider responses only in restricted CheckRun storage with retention controls, never operational logs or student APIs; avoid retaining outbound identity data.

Reserve the maximum priced input/output cost atomically before every provider dispatch, including retries. Count reserved plus settled spend against the lab cap. Reconcile only known usage; retain conservative reservations after unknown outcomes. Use model-specific output caps and versioned pricing. If price or tokenizer safety bound is unknown, refuse dispatch. Rechecks consume the same lab budget; budget exhaustion still allows the TA to complete the lab manually.

Alternatives considered: regex-only ban detection produces false deductions on trap cases; unconstrained model judgment fails evidence and cost requirements. Zero-cost fake usage on timeout creates a parallel overspend path.

### 7. Separate processing completion, queue obligations, and TA decisions

For a stable generation with current Accepted results, create the required set from every violation/unsure plus `ceil(passCount / 10)` randomly selected distinct pass pairs. Persist the random selection and generation; reads do not redraw it. Changing counted inputs/revisions regenerates the sample. Decisions on an unchanged finding revision remain valid if the finding remains applicable. Newly sampled undecided passes require review. The original source verdict remains recorded even when the TA overrides it; overrides do not rewrite the sampling population.

For allowed constraint edits, invalidate every finding/decision for the affected constraint, including deduction-only edits; recheck current Accepted inputs. A deleted constraint requires no new result but retires its queue obligation. A revised test result invalidates downstream findings for that result. Recompute readiness from expected inputs and current results, not a cached displayed count.

TA decision mutations require the displayed finding revision and append an audit event. Deterministic group approval sends explicit IDs/revisions and rejects the whole group if any are stale. Model findings have individual decisions; unsure requires an opened evidence view. All positive deductions require valid original evidence, including manual overrides of pass.

Alternatives considered: sampling by student misses the FAQ's per-constraint unit. Sampling on page load allows refresh to evade review. Treating an empty queue as processing completion can publish while workers are still running.

### 8. Introduce a production contract without changing PA#2 semantics

The unversioned v1.1 endpoint evaluates replay before role/ownership and has no processing-incomplete state. Its contract states that successful publication changes only publication metadata. A production operation also needs current authorization before replay and an atomic grade snapshot. These differences justify `/v2/labs/{labId}/publish` rather than silently changing the existing contract/tests.

Keep the old composition root available to PA#2 tests and an explicitly isolated compatibility/demo process. The production application registers only the versioned publication controller and never shares production data with the legacy handler. The historical tag and contract remain reproducible. This is a production cutover, not an unannounced mutation of the legacy protocol.

Production evaluation order: authenticate session; validate UUID/key/empty body; current TA role and class ownership; stored scoped result; existing publication; deadline; finalized admissions/roster/counted inputs and processing/sample readiness; required decisions; transaction commit. Recheck current authorization under the transaction if ownership changed since the initial read. Auth/input failures are not cached. Completed authorized domain 200/409 results are cached; 5xx are not. Same-key deterministic rejection remains a rejection after conditions improve; the UI must create a new key for a new deliberate attempt.

Lock the lab and active grading generation; concurrent authoring/rejudging/review mutations take that same ordering lock. In one transaction compute the grade versions, insert deductions and publication metadata, store the safe replay response, append the audit record, and enqueue notification intents. Unique publication-per-lab and idempotency-scope constraints enforce cross-process exclusion. Retain completed keys for at least 24 hours and publication metadata indefinitely under the course data-retention policy.

Proposed common API conventions: RFC 9457 with the existing absolute problem-type namespace, concealed 404 for inaccessible resources, 400 for malformed input, 409 for stale revision/key mismatch or invalid business state, 413 for size limits, and 429 for rate limits. Each new contract is documented and approved before endpoint implementation; this planning change does not edit `docs/contract/publish-lab.md`.

| Production resource family | Planned operations | Important boundary |
| --- | --- | --- |
| `/v2/auth/*` | register, login, refresh, logout; later reset and Google callback | No role self-elevation, no account enumeration |
| `/v2/classes/*` | create, assign TAs, enroll, roster import/list | Current role/class ownership, 100-student cap |
| `/v2/labs/*` | authoring, tests, constraints, revisions, readiness | Revision checks and lifecycle restrictions |
| `/v2/labs/{id}/submissions` | accept and list authorized submissions | Durable key/fingerprint, receipt deadline |
| `/v2/submissions/{id}` | results, source for authorized actor, history | Own work or assigned TA only |
| `/v2/labs/{id}/reviews` | queue, evidence, decisions, deterministic group decision | Current finding revision and full audit |
| `/v2/labs/{id}/publish` | whole-lab publication | Production gates and atomic visibility |
| `/v2/labs/{id}/grades` | private results, TA gradebook/export, individual adjustment | Publication visibility and authorization |
| `/v2/deductions/{id}/appeals` | student creation, TA resolution | Unique deduction, window, reason |
| `/v2/events` | authenticated SSE updates | User/class filtering and reauthorization |

### 9. Keep grades and appeals deterministic under corrections

Use original grade snapshots plus append-only grade versions. Deductions are unique by published counted submission and constraint; source evidence and revision are immutable. The publication roster includes members frozen at the deadline; no-submission entries receive zero. Pre-publication TA reports explicitly label provisional grades.

A reasoned manual score is represented as a signed adjustment from the current formula grade, with its requested target and previous value recorded. The effective grade is `clamp(formulaGrade + manualAdjustment, 0, 10)`. For non-AC work formulaGrade is zero. A new manual correction replaces the current adjustment rather than stacking accidental absolute overrides. An accepted appeal removes one deduction and recomputes the formula while retaining the explicit adjustment, so it never lowers the grade merely because a deduction was removed. This adjustment composition is a proposed product policy for a case the sources leave unspecified; expose the breakdown to the TA and test it explicitly.

Appeal creation uses a unique published deduction key, authenticated ownership, nonempty explanation, and server UTC time strictly before publishedAt plus seven days. Use an idempotency key to recover transport retries of the same creation. Resolve pending appeals with optimistic revision checks under the grade lock, changing deduction, grade, appeal, audit, and notification intent together. Rejection does not change the grade; later TA corrections preserve the appeal outcome/history. No bulk unpublish or retroactive constraint mutation is introduced.

### 10. Deliver authenticated views and events without a second state authority

Next.js server components render authenticated page shells; small TanStack Query/client boundaries own submissions, live results, review actions, and appeal forms. Preserve theme tokens, square controls, responsive rules, visible focus, and reduced-motion behavior. Read the installed Next.js guidance before implementation as required by `apps/web/AGENTS.md`.

Use authenticated SSE for one-way progress and notification hints. Persist event IDs in the outbox and filter by current recipient membership before delivery; periodically reauthorize long-lived connections and close them on access loss. Reconnect uses last event ID when available and always refetches authoritative resource state. Events carry safe identifiers/status, not source or hidden evidence. Do not claim exactly-once network delivery; deduplicate by event ID and resource revision. Query keys include identity/class and are cleared on logout.

Public demonstration mode continues using synthetic data. Live evidence never appears in public server caches. Student APIs use explicit projection DTOs rather than serializing internal Finding/CheckRun rows. Emails contain notification summaries and authenticated links only. The outbox provides unique in-app notification IDs and provider idempotency where available; SMTP/provider ambiguity is reported rather than falsely promising exactly-once email.

### 11. Gate evaluator changes without exposing credentials to candidate code

Proposed CLI: `npm run checker -- --data <dataset-dir> --predictions <output.jsonl>`. The worker and CLI call the same pure checker core. Keep the lecturer's dataset/scorer pinned and unmodified, the team set separate, and source digests in each evaluation manifest. The scorer command remains `node eval.mjs --data <dir> --predictions <file> --json <report>` and fails on both exit 1 and 2. The prediction process has no access to labels; a separate trusted scoring step receives them.

Root unit/contract suites use fake model adapters. Live evaluation runs through an approved trusted broker with tightly bounded model requests, pricing, and spend. Candidate PR code runs without credentials, labels, deployment access, or unrestricted network; only a limited relay to the trusted broker is available. The broker never executes candidate code and binds its request log to the evaluated revision. Trusted scoring attests dataset/model/prompt/source hashes. CI accepts only evidence produced by the trusted workflow for the relevant current revision; a PR-provided JSON report cannot self-certify success.

Before the live gate is operational, checker-affecting changes can be developed on feature branches but cannot be claimed evaluation-complete. Branch-protection and CI permission updates require owner approval at implementation time. A harmless intentional failing prediction fixture can prove the scoring gate blocks merges without corrupting lecturer labels or running a paid model solely to fail.

Alternatives considered: handing a provider key to a PR runner violates the trust boundary. Reusing cached model outputs from another prompt or relevant code revision fails to evaluate the actual change. Full live evaluation on unrelated UI-only commits adds cost without new evidence; reuse is allowed only when a trusted attestation verifies the complete relevant-input digest is unchanged.

### 12. Phase the work and account for every source feature

Phases are dependency gates, not claims about current completion or a guarantee of meeting historical upstream weekly dates. Phase 0 resolves concrete approval/contract prerequisites. Phases 1-4 produce the PA#3 path; phases 5-6 complete the PA#4 review/publication/evaluation gates; phases 7-8 cover PA#5 and operational acceptance; phase 9 completes included enhancements. Recheck E6 is promoted into the core pipeline by FAQ-07.

| Phase | Deliverable and exit evidence |
| --- | --- |
| 0 | Approved dependency/contract inventory, pinned source/eval inputs, concrete acceptance matrix |
| 1 | Durable foundation, account/session and class access with adversarial ownership tests |
| 2 | Versioned lab authoring and constraint/test policies visible in TA/student screens |
| 3 | Durable submissions, isolated judging, live private results, crash/deadline evidence |
| 4 | Real checker adapter and CLI, guardrails, rechecking, bounded-cost authorized evaluation |
| 5 | Live human review, pass sampling, revision conflicts, deterministic grade computation |
| 6 | Durable production publication and required evaluation gate with rejection/repair evidence |
| 7 | Student results, seven-day appeals, corrections, and atomic concurrency evidence |
| 8 | Telemetry, red-team deployment, 80-submission measurements, runbook, E2E, post-mortem |
| 9 | Remaining Should enhancements, integrated and verified without relaxing core gates |

Feature traceability (each source feature appears once):

| Source IDs | Status in this plan | Capability / phase |
| --- | --- | --- |
| A1, A2, A3 | Core additions | identity-and-classes / 1 |
| A4, A5 | Included enhancements | identity-and-classes / 9 |
| B1, B2 | Core additions; both enrollment paths and CSV | identity-and-classes / 1 |
| B3 | Deferred Could | Classroom roster import |
| C1, C2, C3, C4, C5 | Core additions | lab-authoring / 2 |
| C6, C7, C8 | Included enhancements | lab-authoring / 9 |
| C9 | Deferred Could | Classroom assignment creation |
| D1, D2, D3, D4, D5 | Core additions | submission-judging / 3 |
| D6, D7 | Included enhancements | submission-judging / 9 |
| E1, E2, E3, E5 | Core additions | constraint-checking, checker-evaluation / 4 |
| E4 | Core addition | review-decisions / 5 |
| E6 | Promoted from Should because FAQ-07 requires it | constraint-checking / 4 |
| E7 | Included enhancement | checker-evaluation / 9 |
| E8 | Core addition | checker-evaluation, quality-gate / 4, 6 |
| F1, F2, F3, F4 | Core additions | review-decisions, grading-and-appeals / 5 |
| F5 | Extend existing PA#2 behavior through v2 | lab-publishing / 6 |
| F6, F7, F8 | Included enhancements | grading-and-appeals / 9 |
| F9 | Deferred Could | Classroom grade export |
| G1, G2, G3 | Core additions | grading-and-appeals / 7 |
| G4 | Included enhancement | grading-and-appeals / 9 |
| G5 | Deferred Could | Free-form inline TA comments |
| H1, H2 | Deferred Could | Cross-lab analytics and student progress |
| I1, I2, I3 | Existing PA#2 baseline retained and extended | quality-gate / all phases |
| I4, I5, I6, I7, I8 | Core additions | service-operations / 8 |
| I9 | Deferred Could | Scheduled daily backup automation |

FAQ traceability:

| FAQ | Owning capability and relevant decision |
| --- | --- |
| 01 | identity-and-classes; current membership checks (2) |
| 02, 03 | submission-judging; receipt, order, admission barrier (4) |
| 04 | grading-and-appeals; non-AC zero/manual adjustment (9) |
| 05 | lab-authoring, submission-judging; execution policy (5) |
| 06, 07 | lab-authoring, constraint-checking; notice/revision invalidation (3, 6, 7) |
| 08 | grading-and-appeals; student projection (10) |
| 09, 10 | review-decisions; evidence and persisted sampling (7) |
| 11 | grading-and-appeals; one deduction per constraint (9) |
| 12 | lab-publishing, grading-and-appeals; transaction and corrections (8, 9) |
| 13, 14 | grading-and-appeals; unique deduction appeal and seven-day window (9) |

The official milestone dates reported by the proposal are PA#3 November 25, PA#4 December 2, and PA#5 December 16, 2026 at 22:00 Asia/Saigon. They are reference checkpoints, not new user-imposed deadlines for this change.

## Risks / Trade-offs

- **Umbrella scope exceeds a single sprint** -> Use the phase exits and one tracked issue checklist; keep PRs bounded by dependency and retain the full requirement map rather than declaring partial work complete.
- **Native parser and sandbox behavior varies by host** -> Exercise the exact pinned parser and Linux execution backend against trap/attack cases before enabling it; unsupported analysis becomes unsure.
- **PostgreSQL job/object workload may need separation** -> Measure the 80-submission workload and queue age; preserve ports so a later broker/object adapter can be introduced based on evidence.
- **Repeated preliminary checking can exhaust USD 0.25** -> Prefer final counted checking after the deadline, reuse only identical revision results, and budget every permitted early check. Manual review remains available after exhaustion.
- **Sanitization cannot infer every personal identifier perfectly** -> Use known identity data plus reviewed detection cases and fail closed when transformation cannot be trusted; never claim a comment-only stripper anonymizes all data.
- **Grade changes, new findings, and publication can race** -> Shared lab/generation locking and revision checks; fault injection at commit boundaries and two-process tests.
- **Live evaluation is expensive and untrusted code is dangerous** -> Trusted broker and evidence verifier, explicit paid-run cap, no live secrets in candidate execution.
- **Source hierarchy contains gaps** -> The baseline decisions on KiB/MiB units, exact deadline exclusion, immutable post-opening deadlines, manual-adjustment composition, and no post-publication bulk rejudge are explicit here. Reconcile the class brief before release; source changes require reviewed deltas rather than unnoticed interpretation changes.
- **Current evidence is PA#2-specific** -> Preserve its tag and artifacts; do not reuse its self-assessment as proof of PA#3-PA#5 functionality.

## Migration Plan

1. Land this planning change through normal review. Before implementation actions requiring approval, present the concrete dependency list and new production contracts. Preserve the existing v1.1 contract/tests and PA#2 tag.
2. Add production composition and additive schema migrations. In-memory fixtures remain available only to deterministic tests/compatibility mode. There is no real PA#2 student database to migrate; do not promote synthetic rows as real users or grades.
3. Bring up an isolated staging environment with synthetic accounts. Verify acceptance, stale results, review, transactional publication, and appeal races before enabling live model use or any external deployment.
4. Run explicitly authorized budgeted evaluation and enable the evaluation branch check only after its emitted context and fail-closed behavior are verified and owner-approved. Existing required checks remain enforced.
5. Deploy the approved production build with versioned routes, provisioned configuration, execution isolation, monitoring, and a recorded migration/restore checkpoint. Perform authorized synthetic E2E/load/red-team exercises; never submit coursework automatically.
6. Roll back application code only to a version compatible with the additive schema. Pause new acceptance/dispatch during unsafe partial upgrades; drain or fence jobs and resume from durable state. Restore only from a verified checkpoint with explicit destructive-action authorization. Never unpublish committed grades to simulate rollback; correct individual outcomes with audit history.

## Open Questions

These choices are safely deferrable within the specified ports and do not change the capability or task breakdown:

- Which approved model/version meets the fixed thresholds within USD 0.25 on measured counted-submission inputs? Record pricing and evidence during the authorized phase-4 evaluation.
- Which approved hosting and email providers fit the deployment/runbook constraints? Select before provisioning without changing public workflow semantics.
- What measured latency targets should become operational SLOs after the first 80-submission baseline? The current acceptance requires complete measurement and no loss/duplication, not an invented course p95 threshold.
