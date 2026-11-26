# Self-assessment — PA#3

Submitted by:

- 23120045 — Dương Khánh Khải Hưng
- 23120124 — Nguyễn Minh Hiếu
- 23120175 — Huỳnh Thái Toàn

Total we claim: 77 / 100

| Criterion                       | Max | We claim | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------------------------------- | --: | -------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| End-to-end deployed flow        |  15 |        0 | No public deployment was provisioned (owner decision 2026-11-24), so the criterion's deployed flow is not met. The same flow is evidenced only locally and in CI: [`docs/evidence/pa3-walkthrough.md`](docs/evidence/pa3-walkthrough.md), `TestHTTPIngressToJudgedResultStaysOwnerPrivate` and `TestMergedPipeline` in [`apps/backend/internal/flow/flow_test.go`](apps/backend/internal/flow/flow_test.go). `deploy/` is prepared and locally verified, not hosted.                                                                                                                                                                                                                                                                                                             |
| Queue and sandbox               |  20 |       18 | Separate leased worker with attempt fences and bounded retries: `TestStopSubmitRestartStoresExactlyOneVerdictSet`, `TestTwoWorkersJudgeOneSubmissionExactlyOnce` ([flow tests](apps/backend/internal/flow/flow_test.go)). Nine real container attacks in [`apps/backend/internal/sandbox/live_test.go`](apps/backend/internal/sandbox/live_test.go) (`TestNetworkIsolation`, `TestCPULimit`, `TestWallDeadlineRemovesTheContainer`, `TestMemoryLimitIncludesSwapAndReadback`, `TestProcessCeilingDoesNotAffectAnotherRun`, `TestOutputCollectionStaysBounded`, `TestNonRootIdentity`, `TestNoHostMountOrDockerSocket`, `TestSecretsStayOutOfEnvFilesAndIncludes`). Deducted 2 because the `pa3` tag run of the `Sandbox attacks` job did not exist when this report was written. |
| Hard rules first                |  15 |       14 | Syntax-aware libclang analyzer with original-line evidence and a trap suite ([`apps/backend/internal/checker/hardrule`](apps/backend/internal/checker/hardrule)): student function named `sort`, comments/strings, aliases, `using`, macros and wrappers are covered. Model-off team cases matched the independent labels exactly, 10/10 ([`evaluation/README.md`](evaluation/README.md#team-cases-task-73)); the committed dev run reports **0 wrongful deductions** ([`evaluation/dev-run/README.md`](evaluation/dev-run/README.md)).                                                                                                                                                                                                                                          |
| Semantic model                  |  20 |       18 | Real NVIDIA NIM run committed with 102 predictions, unmodified scorer report and a call log containing provider request IDs, model, tokens, elapsed time, cost and prompt hashes ([`evaluation/dev-run/`](evaluation/dev-run/README.md)). Comments and known identities are removed before dispatch, unsafe transforms become unsure ([`apps/backend/internal/semantic/sanitize.go`](apps/backend/internal/semantic/sanitize.go)); strict schema/evidence validation and no model-chosen deduction are covered by `TestDuplicateMissingAndUnknownConstraintsFailSchema`, `TestFabricatedEvidenceBecomesUnsure` and `TestInvalidSchemaBecomesUnsureWithoutUnboundedRetry`. Deducted 2 for 78.4% recall (below the later PA#4 90% target), reported as measured.                   |
| Initial limits and review queue |  10 |        9 | Timeout/schema failure becomes unsure with no live provider (`TestTimeoutBecomesUnsureWithoutNetwork`); atomic USD 0.25 reservations include retries, rechecks and concurrency (`TestConcurrentReservationsCannotOverspend`, `TestRetryStopsWhenTheSecondReservationDoesNotFit`). Findings are assigned-TA-only; decisions are revision-bound, append-only and never overwrite checker provenance (`TestSecondDecisionOnTheSameRevisionConflicts`, `TestApplyDecisionKeepsCheckerProvenance`). Students get a safe projection with no deduction, and production Publish is unavailable ([`apps/backend/internal/httpapi/reviews_test.go`](apps/backend/internal/httpapi/reviews_test.go)).                                                                                       |
| Checker and first eval          |  10 |        9 | Dataset-independent command and Dockerfile with `MODEL_OFF=1` before provider setup, run network-none with memory/PID limits ([`apps/backend/checker/README.md`](apps/backend/checker/README.md)); image test `TestImageMatchesTeamCaseLabels` matched all ten team-case predictions with zero calls. Five labelled team cases include trap and injection tags ([`evaluation/team-cases`](evaluation/team-cases)). Official bytes are preserved by the pinned manifest and re-fetch verification (128/128, no redistribution), not vendored; deducted 1 for that reconciliation.                                                                                                                                                                                                 |
| Tests first and focused history |   5 |        4 | Implementation landed in focused commits with tests before or beside the code, and AI-LOG records deliberate faults that failed the intended test and were restored (D1-D14, [`AI-LOG.md`](AI-LOG.md)). Deducted 1 because the two parallel implementations and the later adoption merge (D14) make the history harder to read than a single focused line.                                                                                                                                                                                                                                                                                                                                                                                                                       |
| AI log                          |   5 |        5 | [`AI-LOG.md`](AI-LOG.md) records tools and concrete changed/rejected decisions for every part (D13 approval, D14 adopting `main` and porting only unique evidence), including the fixes found during the real run and the refusal to invent deployment or tag evidence.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

| **Total** | 100 | **77** | Evidence map: [`PA3.md`](PA3.md). |
| --------- | --: | -----: | --------------------------------- |

## What we did not manage

- **No public deployment.** The owner decided on 2026-11-24 to skip external
  provisioning, so there is no URL, no uptime, no hosted synthetic-account
  walkthrough and no fallback recording. Criterion 1 is claimed as 0. The
  deployment stack itself is prepared and locally verified in `deploy/`, and
  the worker constraint (a Docker-capable VM, not a free PaaS) is documented.
- **No `pa3` tag run at the time of writing.** The tag and its `Sandbox attacks`
  job are part of the release procedure; this report does not claim a run that
  had not happened. The ordinary CI job already executes the same nine attack
  tests.
- **No in-repository upstream bytes.** The pinned evaluation tree has no
  redistribution licence, so the submission carries the hash manifest, the
  verifier and the committed prediction run instead. The marker re-fetches and
  verifies the 128 files.
- **The online worker's semantic checks are offline.** Worker findings for
  semantic constraints are `unsure`; the real model path is the separately
  authorized checker run, not the worker.
- **First-run recall is 78.4%.** Eight semantic violations were labelled but
  predicted `pass`. We report the number rather than tuning labels or cases;
  PA#3 has no overall evaluation threshold, but this would not pass the later
  PA#4 90% target.

## What we would do differently

- Decide the hosting and spend plan before implementing the judge, so the
  deployment and the walkthrough recording are part of the milestone rather
  than a last-day gap.
- Avoid two parallel implementations. The reconciliation in D14 cost time and
  made the commit history harder to audit; one branch from the start would have
  been cheaper.
- Freeze scope earlier: the Press proof restyle and the Next.js upgrade were
  valuable but competed with the release evidence.
