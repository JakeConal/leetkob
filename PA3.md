# PA#3 Evidence Index

This file is the evidence map for the `pa3` submission of LeeTKOB. The tag is
checked out and verified with the commands in the repository README, the
backend README and this file.

> **Azure release:** the synthetic assessment service is hosted at
> <https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live> in the
> shared student subscription's `jenkins_group`. Its actual first deployment
> is November 28, 2026, before the updated November 29, 22:00 Asia/Saigon deadline.
> Hosting consumes Toàn’s confirmed student credits, with the spending limit On
> and USD 50 monthly notification budget. Online semantic checks now use bounded
> NVIDIA NIM calls, verified by the [official live walkthrough](docs/evidence/pa3-review/live-model/README.md).
> The first full real-model evaluation remains separate, unchanged evidence.

## Repository and protected branch

- Repository: <https://github.com/JakeConal/leetkob>
- Protected branch: `main`
- Protection API: <https://api.github.com/repos/JakeConal/leetkob/branches/main/protection>
- Captured settings: [`docs/evidence/main-branch-protection.json`](docs/evidence/main-branch-protection.json)
- Required checks: `Lint`, `Format`, `Typecheck` and `Test`, with strict
  up-to-date enforcement and no bypass actors. The November 28 read-only
  recapture reports zero required approvals; earlier captures are historical.
  This release changes no protection setting and requests both teammates for review.
- Pull requests: [#39](https://github.com/JakeConal/leetkob/pull/39) (Go
  foundation), [#41](https://github.com/JakeConal/leetkob/pull/41)
  (submission, judging, review), [#44](https://github.com/JakeConal/leetkob/pull/44)
  (checker, real evaluation, deployment artifacts),
  [#45](https://github.com/JakeConal/leetkob/pull/45) (security upgrade) and
  [#46](https://github.com/JakeConal/leetkob/pull/46) (reviewer UI).

## Submitted tag and CI

- Release tag: `pa3`. Verify the remote tag and matching run before treating
  an archive as the final release: `git rev-parse pa3^{commit}`.
- CI run for the tag: [Quality Gate runs filtered to `pa3`](https://github.com/JakeConal/leetkob/actions/workflows/quality.yml?query=branch%3Apa3).
  The matching run's head SHA must equal
  `git rev-parse pa3^{commit}`. A commit cannot
  contain the ID of the run that its own tag push triggers, so the filtered
  list is linked rather than a single run ID (the same convention as
  [`PA2.md`](PA2.md)).
- The submission tag triggers the additive job **`Sandbox attacks`**, which runs only for
  `refs/tags/pa3`. It builds the sandbox and bundled-analyzer checker images, runs the
  nine real container attacks, and fails on `--- SKIP` or on any provider
  credential in the environment. Definition:
  [`.github/workflows/quality.yml`](.github/workflows/quality.yml).
- Green reviewed-implementation evidence: [run 46128979287](https://github.com/JakeConal/leetkob/actions/runs/46128979287)
  on `b8a0b53`, the application revision exercised by the live walkthrough.
  Final merge/tag checks must match the release revision as described above.
- Earlier pre-tag evidence: [run 45672213230](https://github.com/JakeConal/leetkob/actions/runs/45672213230)
  on `main` (`aeed538`) and [run 45306355723](https://github.com/JakeConal/leetkob/actions/runs/45306355723)
  on PR #44, which executed the merged pipeline and sandbox tests.

## Synthetic accounts

These deliberately public identities contain only synthetic assessment data.
They grant no Azure, GitHub or model-provider access. Sign in through `/live`.
The provisioner hashes these synthetic passwords before storage and is idempotent;
it never silently resets an existing password or seeds the marked lab.

| Account                              | Role    | Class       | Synthetic password                           |
| ------------------------------------ | ------- | ----------- | -------------------------------------------- |
| `admin@assessment.leetkob.test`      | admin   | none        | `Synthetic-PA3-Demo-2026!Admin`              |
| `ta-a@assessment.leetkob.test`       | TA      | PA3 Class A | `Synthetic-PA3-Demo-2026!TeachingAssistantA` |
| `ta-b@assessment.leetkob.test`       | TA      | PA3 Class B | `Synthetic-PA3-Demo-2026!TeachingAssistantB` |
| `student-a1@assessment.leetkob.test` | student | PA3 Class A | `Synthetic-PA3-Demo-2026!StudentA1`          |
| `student-a2@assessment.leetkob.test` | student | PA3 Class A | `Synthetic-PA3-Demo-2026!StudentA2`          |

## Deployment

- URL: <https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>
- Host: Ubuntu 24.04 image `24.04.202611170`, B2als v2 (2 vCPU / 4 GiB),
  64 GiB Standard SSD, Docker 29.10.1, Southeast Asia. Only TCP 80/443 is public.
- Repeatable configuration, costs, recovery, rollback and security review:
  [`deploy/AZURE.md`](deploy/AZURE.md). The approximately USD 42.91/month
  baseline excludes variable disk operations/egress and consumes student credits.
- Actual-host sandbox qualification: [`sandbox.txt`](docs/evidence/pa3-azure/sandbox.txt),
  [`checker-image.txt`](docs/evidence/pa3-azure/checker-image.txt),
  [`host.json`](docs/evidence/pa3-azure/host.json). All nine attacks passed without skips.
- Actual public verification and fallback recording:
  [`docs/evidence/pa3-walkthrough.md`](docs/evidence/pa3-walkthrough.md).
- The base Compose file selects `MODEL_OFF=1` and isolated analysis. The
  [model overlay](deploy/docker-compose.model.yml) enables bounded real calls
  after [private operator setup](deploy/NIM.md). The live overlay is active:
  [three actual calls and runtime readback](docs/evidence/pa3-review/live-model/README.md)
  prove deployed integration. The trusted worker alone sees
  the Docker socket; source parsing and compilation happen in qualified containers.

The service is intended to remain up until PA#4 marks. Restart policies are
configured, but future availability depends on the owner's credit expiry and
operation of the VM. No earlier uptime is claimed. The current deployment runbook is `deploy/AZURE.md`.

## End-to-end flow

- Configuration and observed results:
  [`docs/evidence/pa3-walkthrough.md`](docs/evidence/pa3-walkthrough.md).
- API/worker/checker entry points: `apps/backend/cmd/{api,worker,checker}`; the
  worker uses leased durable jobs and a separate sandbox controller.
- Flow tests: `TestMergedPipeline`,
  `TestHTTPIngressToJudgedResultStaysOwnerPrivate`,
  `TestStopSubmitRestartStoresExactlyOneVerdictSet` and
  `TestTwoWorkersJudgeOneSubmissionExactlyOnce` in
  [`apps/backend/internal/flow/flow_test.go`](apps/backend/internal/flow/flow_test.go).
  The cross-class TA and other-student reads receive the concealed 404.

## Queue and sandbox controls

`apps/backend/internal/sandbox/live_test.go` exercises the nine rubric
minimums with real containers:

| Control               | Test                                        |
| --------------------- | ------------------------------------------- |
| No network            | `TestNetworkIsolation`                      |
| CPU                   | `TestCPULimit`                              |
| Wall time (container) | `TestWallDeadlineRemovesTheContainer`       |
| Memory including swap | `TestMemoryLimitIncludesSwapAndReadback`    |
| Processes             | `TestProcessCeilingDoesNotAffectAnotherRun` |
| Output                | `TestOutputCollectionStaysBounded`          |
| Non-root              | `TestNonRootIdentity`                       |
| No host mounts/socket | `TestNoHostMountOrDockerSocket`             |
| No secrets            | `TestSecretsStayOutOfEnvFilesAndIncludes`   |

Compilation runs in the same isolated image; `TestImageProvenanceAndIsolatedHello`
checks the compiler and pinned packages. Verdict mapping (AC/WA/TLE/CE/RE,
memory reported as RE, infrastructure failure separate) is in
[`apps/backend/sandbox/README.md`](apps/backend/sandbox/README.md).

## Hard rules and semantic model

- Hard rules: libclang analyzer `apps/backend/native/cpp-analyzer`, resolved
  declarations and original-line evidence, with the trap suite in
  `apps/backend/internal/checker/hardrule`. Comment/string mentions, student
  functions named `sort`, aliases, `using`, macros and wrappers are covered.
- Sanitization: lexer-aware comment/identity removal with line mapping and
  unsafe-input refusal (`apps/backend/internal/semantic/sanitize.go`).
- Semantic calls: strict schema/evidence validation, one bounded repair,
  timeout/unknown causes become `unsure`, and no score/deduction field is
  accepted (`apps/backend/internal/semantic`).
- Budget: atomic conservative reservations against the unchanged USD 0.25
  per-lab cap, including retries, rechecks and concurrency
  (`apps/backend/internal/semantic/budget_test.go`).
- Privacy: findings are visible only to the assigned TA; students receive a
  safe judge projection and no deduction. Production Publish is unavailable
  for PA#3 and the PA#2 compatibility Publish stays on fixture-only storage.

## Checker and first evaluation

Generic checker image and command:

```bash
docker build -f apps/backend/checker/Dockerfile -t leetkob-checker:pa3 apps/backend
checker_inputs=$(mktemp -u /tmp/leetkob-checker-inputs.XXXXXX)
node scripts/evaluation/sources.mjs stage evaluation/team-cases "$checker_inputs"
checker_out=$(mktemp -d /tmp/leetkob-checker.XXXXXX)
chmod 0777 "$checker_out"  # synthetic predictions only; uid 65534 writes here

docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 \
  --memory 512m --memory-swap 512m --user 65534:65534 \
  -v "$checker_inputs:/data:ro" -v "$checker_out:/out" \
  -e MODEL_OFF=1 \
  leetkob-checker:pa3 --data /data --predictions /out/predictions.jsonl
```

`MODEL_OFF=1` is the default and is selected before any provider client or
credential exists; semantic pairs are `unsure`, and every ban-list pair is
decided by the bundled analyzer. On 2026-11-24 the image produced 10
predictions for the five team cases (ten solutions) that matched the
independent labels exactly with zero model calls
([`evaluation/README.md`](evaluation/README.md#team-cases-task-73)).

Committed first real-model run:
[`evaluation/dev-run/`](evaluation/dev-run/README.md) — 102 predictions,
**0 wrongful deductions**, 29/37 violations caught (78.4% recall), 4 unsure
(3.9%), unmodified scorer exit 1 (valid for PA#3; exit 2 would be invalid).
The call log records provider request IDs, model, tokens, elapsed time, cost
and prompt hashes; prompts, source, identities and the key are not logged.
A lab of 80 submissions is ≈97.5k tokens and USD 0.00 at the free NVIDIA tier;
the USD 0.25 cap is reserved before every dispatch.

Official bytes: all 128 unchanged files are included in
[`evaluation/official/`](evaluation/official/) under the lecturer’s course hand-in
instruction, with [separate provenance](evaluation/official-provenance.md).
`npm test` verifies every published SHA-256 before the test suites run. This
course submission does not claim an upstream open-source redistribution license.

## Tests-first history and AI log

- Focused implementation commits are kept in order on `main`; the AI log
  records the real changed/rejected decisions, including deliberate faults
  that were detected and restored (D1-D14 in [`AI-LOG.md`](AI-LOG.md)).
- No test, label, eval case or threshold was changed to obtain green checks.
  The first-run recall is reported as measured.

## Rubric evidence map

| Criterion                       | Evidence                                                                                                            |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| End-to-end deployed flow        | Public HTTPS flow, actual-host qualification, restart/privacy proof and the official three-case live NIM recording. |
| Queue and sandbox               | Leased worker, fence/restart tests, nine sandbox attack tests, `pa3` tag job.                                       |
| Hard rules first                | Analyzer trap suite, model-off team cases (10/10), dev-run with 0 wrongful hard-rule deductions.                    |
| Semantic model                  | Guarded adapter, sanitizer, budget, committed `evaluation/dev-run/` predictions, scorer report, call log.           |
| Initial limits and review queue | Schema/timeout unsure tests, review decision tests, private projections, no production Publish.                     |
| Checker and first eval          | Dataset-independent image/command, `MODEL_OFF=1`, team cases, unchanged official bytes, honest metrics.             |
| Tests first and focused history | Commit order, AI-LOG fault entries, no weakened checks or labels.                                                   |
| AI log                          | [`AI-LOG.md`](AI-LOG.md) tool/part entries and D1-D14 changed/rejected decisions.                                   |

## Known gaps

- The updated deadline is November 29, 22:00 Asia/Saigon. Deployment began
  November 28; retention until PA#4 marks remains an ongoing obligation, and
  future uptime is not guaranteed by a one-time check.
- Live worker verification is in
  [`docs/evidence/pa3-review/live-model/`](docs/evidence/pa3-review/live-model/README.md):
  all three official cases reached AC; C2 returned pass/violation/violation.
  The 2:06.60 uncut fallback recording includes privacy, TA confirmation and
  blocked publication. The older model-off recordings are historical evidence.
- Recall is 78.4%, below the later PA#4 90% target. PA#3 imposes no overall
  evaluation threshold, and the result is not adjusted. Rescoring on November 28
  reproduced the committed summary exactly, with all 128 official files verified
  before and after scoring.
- SonarCloud's Azure identity/encryption findings remain open for review. The
  standard Quality Gate jobs and actual sandbox results are linked separately;
  this release does not suppress the scanner or claim its PR check is green.
- PR [#50](https://github.com/JakeConal/leetkob/pull/50) carries this release.
  Review was requested from Toanapr and XuXinhNo1; agent review is not represented
  as teammate approval. The owner authorized normal merge after validation,
  `pa3` replacement and a clean archive. Verify the actual PR state, remote tag
  and matching tag checks before using the ZIP. Coursework upload is not performed.
