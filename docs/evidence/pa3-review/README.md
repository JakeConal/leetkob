# PA#3 final review

Reviewed against the updated rubric, brief and Classroom assignment on November
28, 2026. The current deadline is **November 29, 22:00 Asia/Saigon**; the source
IDs and modification times are in the
[acceptance matrix](../../requirements/acceptance-matrix.md).

PR: [#50](https://github.com/JakeConal/leetkob/pull/50), parent
[#38](https://github.com/JakeConal/leetkob/issues/38). Reviewers Toanapr and
XuXinhNo1 are requested. The owner has authorized merge after review and required
checks. No branch protection or required result name has changed.

| Rubric area                  | Review result and evidence                                                                                                                                                                                                              |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Deployed end-to-end flow     | Public HTTPS, synthetic accounts, privacy and restart proof. The final 2:06.60 official three-case recording uses actual deployed NIM calls; see [live evidence](live-model/README.md).                                                 |
| Queue and sandbox            | Durable leases, fences and race tests; all nine attacks passed on the Azure host and initial tag. Final tag must repeat its Sandbox attacks job.                                                                                        |
| Hard rules first             | Syntax-aware analyzer and trap tests. Full model-off published set decides all 55 ban-list pairs with zero wrongful deductions.                                                                                                         |
| Semantic model               | Fixed production offline stub. Three real deployed requests returned pass/violation/violation with original evidence, known usage/cost and sanitized prompt hashes. Schema, timeout and cap failures use network-free regression tests. |
| Limits and private review    | Persistent USD 0.25 reservations plus zero-price call ceilings; unknown/timeout/schema results are unsure, TA-only queue, revision-bound decisions, safe student projection and blocked production publication.                         |
| Checker and first evaluation | All 128 official files now included unchanged with a mandatory hash guard. Prediction-only staging excludes labels/tests/scorer. Separate real first-run predictions retain 78.4% recall and scorer exit 1.                             |
| Tests and history            | PostgreSQL service added so CI runs durable adapters. Flow fixture now owns required compiler/analyzer images and performs parsing in isolation. Race checks and assertions are preserved.                                              |
| AI log                       | Actual tools, rejected approaches, defect-detection checks and remaining gaps are recorded; no provider ID, uptime or result is invented.                                                                                               |

## Verified regression checks

[CI 46128979287](https://github.com/JakeConal/leetkob/actions/runs/46128979287)
passed Lint, Format, Typecheck, Test, Build and Security at `b8a0b53`, the
application revision used by the live walkthrough. Test ran
against real PostgreSQL; Sandbox attacks is intentionally tag-only and was
skipped for that PR run. Final documentation/merge/tag revisions require their
own matching checks before archiving.

All six root quality commands also passed locally against a dedicated
disposable PostgreSQL instance, with race detection retained. The security
command met its configured high-severity threshold; npm audit still reported
nine moderate development-tool dependency advisories. No dependency, threshold
or required check was changed to hide those findings.

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

## Release procedure and remaining obligations

The private replacement installation, activation and official three-case live
proof are complete: [recording and actual call records](live-model/README.md).
The operator setup initially failed because the relocated `az` launcher had a
missing build-time interpreter. Its regression-first repair now uses the bundled
Python module and checks startup before prompting; all seven offline tests pass.

Merge through normal protected checks, align `pa3`
to the reviewed merged revision, wait for its tag checks and generate a clean
matching archive including `.git`. The previous `_91.zip` is the initial release
and does not represent these review fixes. The final claim is **95/100**, with
deductions explained in the self-assessment. An archive is released only after
the actual merge/tag checks and clean-extraction verification pass.

Keep the service reachable until PA#4 marks. The first full eval's 78.4% recall,
open Sonar findings and moderate development-tool advisories remain disclosed.
No teammate approval, future uptime or PA#4 threshold pass is inferred from
this review. Coursework upload is not authorized.
