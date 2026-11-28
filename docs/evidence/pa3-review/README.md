# PA#3 final review in progress

Reviewed against the updated rubric, brief and Classroom assignment on November
28, 2026. The current deadline is **November 29, 22:00 Asia/Saigon**; the source
IDs and modification times are in the
[acceptance matrix](../../requirements/acceptance-matrix.md).

PR: [#50](https://github.com/JakeConal/leetkob/pull/50), parent
[#38](https://github.com/JakeConal/leetkob/issues/38). Reviewers Toanapr and
XuXinhNo1 are requested. The owner has authorized merge after review and required
checks. No branch protection or required result name has changed.

| Rubric area                  | Review result and evidence                                                                                                                                                                                      |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Deployed end-to-end flow     | Public HTTPS, synthetic accounts, privacy and restart proof exist. The original recording uses model-off; official sort-merge compliant/violation/injection flow with a live model is still required.           |
| Queue and sandbox            | Durable leases, fences and race tests; all nine attacks passed on the Azure host and initial tag. Final tag must repeat its Sandbox attacks job.                                                                |
| Hard rules first             | Syntax-aware analyzer and trap tests. Full model-off published set decides all 55 ban-list pairs with zero wrongful deductions.                                                                                 |
| Semantic model               | Fixed production worker's unconditional offline stub. Bounded real adapter, strict schema/evidence, sanitizer and fake-transport tests exist. Actual deployed provider request remains pending.                 |
| Limits and private review    | Persistent USD 0.25 reservations plus zero-price call ceilings; unknown/timeout/schema results are unsure, TA-only queue, revision-bound decisions, safe student projection and blocked production publication. |
| Checker and first evaluation | All 128 official files now included unchanged with a mandatory hash guard. Prediction-only staging excludes labels/tests/scorer. Separate real first-run predictions retain 78.4% recall and scorer exit 1.     |
| Tests and history            | PostgreSQL service added so CI runs durable adapters. Flow fixture now owns required compiler/analyzer images and performs parsing in isolation. Race checks and assertions are preserved.                      |
| AI log                       | Actual tools, rejected approaches, defect-detection checks and remaining gaps are recorded; no provider ID, uptime or result is invented.                                                                       |

## Verified regression checks

[CI 46122872337](https://github.com/JakeConal/leetkob/actions/runs/46122872337)
passed Lint, Format, Typecheck, Test, Build and Security at `d3b17ff`. Test ran
against real PostgreSQL; Sandbox attacks is intentionally tag-only and was
skipped for that PR run. A later commit needs its own matching checks.

Local race-enabled worker/semantic tests and merged pipeline passed. Deliberately
restoring the offline worker stub failed the configured-provider regression;
disabling the per-lab free-call cap admitted the forbidden third reservation and
failed its regression. Restoring each implementation passed. Labels, fixtures,
required checks and attack controls were not weakened.

The actual full offline run is preserved in [model-off/](model-off/): 102 rows,
55 ban-list decisions, 47 semantic unsure, zero wrongful deductions, zero model
calls, scorer exit 1. It used a network-none, read-only, non-root container with
512 MiB memory/swap and a 64-process ceiling. The scorer and prediction inputs
were kept separate. This verifies the offline path, not live integration.

## SonarCloud review

The optional SonarCloud PR result is red. It is not a required protected-branch
check; no scanner rule is suppressed or green result claimed. Its current
findings were reviewed as follows:

- The digest-plus-tag Docker warning was corrected to digest-only. ARM property
  order was corrected without changing values.
- The fixed PostgreSQL password belongs solely to an ephemeral disposable CI
  fixture. It is public synthetic test data, not the deployment's generated
  database credential. Worker test tokens/identities are also synthetic; HTTP
  and userinfo URLs are intentional negative cases that assert rejection.
- Cognitive complexity, duplicated test constants and source-staging regex
  complexity are maintainability findings. Regression tests cover these paths;
  no runtime security failure is inferred from the issue severity label.
- Findings in `evaluation/official/eval.mjs` concern lecturer-owned immutable
  input. Editing that scorer would violate the byte-integrity requirement.
- Azure location values deliberately enforce the subscription's permitted
  regions. The VM needs no Azure API identity. Its OS disk uses platform-managed
  encryption and public export is disabled. Customer-managed keys and encryption
  at host are separate options, neither claimed as enabled. Azure documents
  [default disk encryption](https://learn.microsoft.com/en-us/azure/virtual-machines/disk-encryption);
  Sonar's [ARM encryption rule](https://github.com/SonarSource/sonar-iac/blob/master/iac-extensions/arm/src/main/java/org/sonar/iac/arm/checks/UnencryptedCloudServicesCheck.java)
  does not treat that default as satisfying its explicit-property checks.

## Remaining release proof

Install a replacement credential privately, activate the reviewed worker, verify
actual model provenance and record the official three-case deployed flow. Then
finalize the score/evidence, merge through normal protected checks, align `pa3`
to the reviewed merged revision, wait for its tag checks and generate a clean
matching archive including `.git`. The previous `_91.zip` is the initial release
and does not represent these review fixes. Coursework upload is not authorized.
