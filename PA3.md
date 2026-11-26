# PA#3 Evidence Index

This file is the evidence map for the `pa3` submission of LeeTKOB. The tag is
checked out and verified with the commands in the repository README, the
backend README and this file.

> **Deployment status: no public deployment exists.** On 2026-11-24 the owner
> decided to skip external provisioning, so this submission claims **no URL,
> no uptime and no fallback recording**. The flow is evidenced by the local
> staging run and by CI instead. `deploy/` contains the repeatable stack that
> was verified locally, and the gaps are disclosed in
> [`docs/evidence/pa3-walkthrough.md`](docs/evidence/pa3-walkthrough.md) and in
> [`SELF_ASSESSMENT_REPORT.md`](SELF_ASSESSMENT_REPORT.md). No link here points
> at a deployment, run or provider request that did not happen.

## Repository and protected branch

- Repository: <https://github.com/JakeConal/leetkob>
- Protected branch: `main`
- Protection API: <https://api.github.com/repos/JakeConal/leetkob/branches/main/protection>
- Captured settings: [`docs/evidence/main-branch-protection.json`](docs/evidence/main-branch-protection.json)
- Required checks: `Lint`, `Format`, `Typecheck` and `Test`, with strict
  up-to-date enforcement, one required approval and no bypass actors.
- Pull requests: [#39](https://github.com/JakeConal/leetkob/pull/39) (Go
  foundation), [#41](https://github.com/JakeConal/leetkob/pull/41)
  (submission, judging, review), [#44](https://github.com/JakeConal/leetkob/pull/44)
  (checker, real evaluation, deployment artifacts),
  [#45](https://github.com/JakeConal/leetkob/pull/45) (security upgrade) and
  [#46](https://github.com/JakeConal/leetkob/pull/46) (reviewer UI).

## Submitted tag and CI

- Tag: `pa3` — created on the commit that contains this file.
- CI run for the tag: [Quality Gate runs filtered to `pa3`](https://github.com/JakeConal/leetkob/actions/workflows/quality.yml?query=branch%3Apa3).
  The **most recent** run in that list is the run for the submitted tag
  commit; its head SHA must equal `git rev-parse pa3^{commit}`. A commit cannot
  contain the ID of the run that its own tag push triggers, so the filtered
  list is linked rather than a single run ID (the same convention as
  [`PA2.md`](PA2.md)).
- The submission tag triggers the additive job **`Sandbox attacks`**, which runs only for
  `refs/tags/pa3`. It builds the sandbox, analyzer and checker images, runs the
  nine real container attacks, and fails on `--- SKIP` or on any provider
  credential in the environment. Definition:
  [`.github/workflows/quality.yml`](.github/workflows/quality.yml).
- Green pre-tag evidence: [run 45672213230](https://github.com/JakeConal/leetkob/actions/runs/45672213230)
  on `main` (`aeed538`) and [run 45306355723](https://github.com/JakeConal/leetkob/actions/runs/45306355723)
  on PR #44, which executed the merged pipeline and sandbox tests.

## Synthetic accounts

`go run ./cmd/provision` creates these identities and two classes. Passwords
are supplied in `ASSESSMENT_PASSWORDS` for one provisioning run, hashed before
storage, and never committed or baked into an image.

| Account                              | Role    | Class         |
| ------------------------------------ | ------- | ------------- |
| `admin@assessment.leetkob.test`      | admin   | none          |
| `ta-a@assessment.leetkob.test`       | TA      | `PA3 Class A` |
| `ta-b@assessment.leetkob.test`       | TA      | `PA3 Class B` |
| `student-a1@assessment.leetkob.test` | student | `PA3 Class A` |
| `student-a2@assessment.leetkob.test` | student | `PA3 Class A` |

## Deployment

Status: **prepared, not provisioned** (owner decision, 2026-11-24). There is no
public URL and no claimed availability. [`deploy/README.md`](deploy/README.md)
documents the stack, the Docker-socket constraint for the worker, the
free-tier hosting proposal, the health/restart/recovery/rollback steps and the
zero-spend ceiling.

Locally verified on 2026-11-24:

```bash
cp deploy/.env.example deploy/.env      # synthetic values, never committed
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.yml up -d --build postgres api web
```

- PostgreSQL 18.7 + API health check returned `{"status":"ok"}`; the web app
  served `/live`; sign-in through the same-origin `/v2` rewrite returned a
  session cookie; provisioning was idempotent on a second run.
- The worker requires a Docker daemon and a same-path host work directory, so
  it cannot run on a free PaaS tier; a Docker-capable VM is required. That
  external step was not authorized or performed.

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

docker run --rm --network none --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 \
  --memory 512m --memory-swap 512m --user 65534:65534 \
  -v "$PWD/evaluation/team-cases:/data:ro" -v /tmp/checker-out:/out \
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

Official bytes: the repository commits the byte-hash manifest
[`evaluation/official-source.json`](evaluation/official-source.json) and the
verifier/stager. The marker re-fetches the pinned commit and verifies all 128
files before and after scoring; upstream bytes are not redistributed because
the pinned tree has no licence. This is the documented reconciliation, not a
claim that the files are vendored.

## Tests-first history and AI log

- Focused implementation commits are kept in order on `main`; the AI log
  records the real changed/rejected decisions, including deliberate faults
  that were detected and restored (D1-D14 in [`AI-LOG.md`](AI-LOG.md)).
- No test, label, eval case or threshold was changed to obtain green checks.
  The first-run recall is reported as measured.

## Rubric evidence map

| Criterion                       | Evidence                                                                                                  |
| ------------------------------- | --------------------------------------------------------------------------------------------------------- |
| End-to-end deployed flow        | Local staging + flow tests above. **No public deployment; criterion not fully met.**                      |
| Queue and sandbox               | Leased worker, fence/restart tests, nine sandbox attack tests, `pa3` tag job.                             |
| Hard rules first                | Analyzer trap suite, model-off team cases (10/10), dev-run with 0 wrongful hard-rule deductions.          |
| Semantic model                  | Guarded adapter, sanitizer, budget, committed `evaluation/dev-run/` predictions, scorer report, call log. |
| Initial limits and review queue | Schema/timeout unsure tests, review decision tests, private projections, no production Publish.           |
| Checker and first eval          | Dataset-independent image/command, `MODEL_OFF=1`, team cases, official byte manifest, honest metrics.     |
| Tests first and focused history | Commit order, AI-LOG fault entries, no weakened checks or labels.                                         |
| AI log                          | [`AI-LOG.md`](AI-LOG.md) tool/part entries and D1-D14 changed/rejected decisions.                         |

## Known gaps

- No public URL, no fallback recording and no claimed uptime. The deployment
  stack is prepared and locally verified only.
- The tag CI run and its `Sandbox attacks` job appear only after `pa3` is
  pushed; nothing in this file asserts that run before it exists.
- The online worker runs semantic checks offline (`unsure`); the real model
  path is the committed checker run, not the worker.
- Recall is 78.4%, below the later PA#4 90% target. PA#3 imposes no overall
  evaluation threshold, and the result is not adjusted.
