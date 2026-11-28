# Self-assessment — PA#3

Submitted by:

- 23120045 — Dương Khánh Khải Hưng
- 23120124 — Nguyễn Minh Hiếu
- 23120175 — Huỳnh Thái Toàn

Total we claim: 91 / 100

| Criterion                       | Max | We claim | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ------------------------------- | --: | -------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| End-to-end deployed flow        |  15 |       12 | Public HTTPS service in Azure `jenkins_group`, five synthetic accounts and actual browser/HTTP walkthrough: [`PA3.md`](PA3.md), [`public evidence`](docs/evidence/pa3-azure/README.md), [`recording`](docs/evidence/pa3-azure/walkthrough.webm). AC/pass, AC/violation with original evidence, injection, WA/CE bypass, private student results, cross-class denial and worker recovery are exercised against the real service. Deducted 3 for first deployment after the November 25 deadline and offline online semantic findings; no earlier uptime is claimed.                                                                                                                                                                                             |
| Queue and sandbox               |  20 |       20 | Separate leased worker with attempt fences and bounded retries; `TestStopSubmitRestartStoresExactlyOneVerdictSet` and `TestTwoWorkersJudgeOneSubmissionExactlyOnce` ([flow tests](apps/backend/internal/flow/flow_test.go)). All nine real container attacks passed without skips on the actual Azure host ([log](docs/evidence/pa3-azure/sandbox.log)); public worker stop/submit/restart and durable verdict checks are recorded. The `pa3` tag triggers the additive Sandbox attacks job; verify the [matching SHA/run](https://github.com/JakeConal/leetkob/actions/workflows/quality.yml?query=branch%3Apa3) with the archive tag.                                                                                                                        |
| Hard rules first                |  15 |       14 | Syntax-aware libclang analyzer with original-line evidence and a trap suite ([`apps/backend/internal/checker/hardrule`](apps/backend/internal/checker/hardrule)): student function named `sort`, comments/strings, aliases, `using`, macros and wrappers are covered. Model-off team cases matched the independent labels exactly, 10/10 ([`evaluation/README.md`](evaluation/README.md#team-cases-task-73)); the committed dev run reports **0 wrongful deductions** ([`evaluation/dev-run/README.md`](evaluation/dev-run/README.md)).                                                                                                                                                                                                                        |
| Semantic model                  |  20 |       18 | Real NVIDIA NIM run committed with 102 predictions, unmodified scorer report and a call log containing provider request IDs, model, tokens, elapsed time, cost and prompt hashes ([`evaluation/dev-run/`](evaluation/dev-run/README.md)). Comments and known identities are removed before dispatch, unsafe transforms become unsure ([`apps/backend/internal/semantic/sanitize.go`](apps/backend/internal/semantic/sanitize.go)); strict schema/evidence validation and no model-chosen deduction are covered by `TestDuplicateMissingAndUnknownConstraintsFailSchema`, `TestFabricatedEvidenceBecomesUnsure` and `TestInvalidSchemaBecomesUnsureWithoutUnboundedRetry`. Deducted 2 for 78.4% recall (below the later PA#4 90% target), reported as measured. |
| Initial limits and review queue |  10 |        9 | Timeout/schema failure becomes unsure with no live provider (`TestTimeoutBecomesUnsureWithoutNetwork`); atomic USD 0.25 reservations include retries, rechecks and concurrency (`TestConcurrentReservationsCannotOverspend`, `TestRetryStopsWhenTheSecondReservationDoesNotFit`). Findings are assigned-TA-only; decisions are revision-bound, append-only and never overwrite checker provenance (`TestSecondDecisionOnTheSameRevisionConflicts`, `TestApplyDecisionKeepsCheckerProvenance`). Students get a safe projection with no deduction, and production Publish is unavailable ([`apps/backend/internal/httpapi/reviews_test.go`](apps/backend/internal/httpapi/reviews_test.go)).                                                                     |
| Checker and first eval          |  10 |        9 | Dataset-independent command and Dockerfile with `MODEL_OFF=1` before provider setup, run network-none with memory/PID limits ([`apps/backend/checker/README.md`](apps/backend/checker/README.md)); image test `TestImageMatchesTeamCaseLabels` matched all ten team-case predictions with zero calls. Five labelled team cases include trap and injection tags ([`evaluation/team-cases`](evaluation/team-cases)). Official bytes are preserved by the pinned manifest and re-fetch verification (128/128, no redistribution), not vendored; deducted 1 for that reconciliation.                                                                                                                                                                               |
| Tests first and focused history |   5 |        4 | Implementation landed in focused commits with tests before or beside the code, and AI-LOG records deliberate faults that failed the intended test and were restored (D1-D14, [`AI-LOG.md`](AI-LOG.md)). Deducted 1 because the two parallel implementations and the later adoption merge (D14) make the history harder to read than a single focused line.                                                                                                                                                                                                                                                                                                                                                                                                     |
| AI log                          |   5 |        5 | [`AI-LOG.md`](AI-LOG.md) records tools and concrete changed/rejected decisions for every part (D13 approval, D14 adopting `main` and porting only unique evidence), including the fixes found during the real run and the refusal to invent deployment or tag evidence.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

| **Total** | 100 | **91** | Evidence map: [`PA3.md`](PA3.md). |
| --------- | --: | -----: | --------------------------------- |

## What we did not manage

- **Deployment started late.** The public service first became reachable on
  November 28, 2026, after the November 25 deadline. Availability from the
  deadline is not met and future uptime depends on student credits and VM
  operation. Hosting and a genuine public recording now exist; no dates are
  backdated. The recorded runtime uses the isolated analyzer, with no parser in
  the trusted worker.
- **Release review remains open.** PR #50 requests Toanapr and XuXinhNo1.
  SonarCloud flags Azure identity/encryption configuration for review; Azure
  confirms platform-managed disk encryption, public disk export is disabled,
  and the VM deliberately has no Azure identity. No rule or check was disabled,
  no green Sonar result is claimed, and the PR is not automatically merged.
- **No in-repository upstream bytes.** The pinned evaluation tree has no
  redistribution licence, so the submission carries the hash manifest, the
  verifier and the committed prediction run instead. The marker re-fetches and
  verifies the 128 files.
- **The online worker's semantic checks are offline.** Worker findings for
  semantic constraints are `unsure`; the real model path is the separately
  authorized checker run, not the worker.
- **First-run recall is 78.4%.** Eight labelled violations were not caught as violations
  (some were predicted `pass`, others `unsure`). We report the number rather than tuning labels or cases;
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

## Release verification

The November 28 independent rescore matched the committed scorer summary exactly:
102 pairs, zero wrongful deductions, 29/37 caught (78.4%) and 4/102 unsure (3.9%).
All 128 pinned official files verified unchanged before and after scoring.
No new model request, label change or threshold change was made for the release.
The actual-host qualification and public reports are linked from
[`docs/evidence/pa3-azure/README.md`](docs/evidence/pa3-azure/README.md).

The final archive is named `23120045-23120124-23120175_91.zip`, includes a real
`.git` directory and the `pa3` tag, and is verified by clean extraction. Coursework
upload is not performed. The index links the tag-filtered CI list because a
commit cannot contain the run ID triggered by pushing its own tag; match the
run's head SHA to `git rev-parse pa3^{commit}`.
