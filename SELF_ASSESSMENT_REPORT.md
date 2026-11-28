# Self-assessment — PA#3

Submitted by:

- 23120045 — Dương Khánh Khải Hưng
- 23120124 — Nguyễn Minh Hiếu
- 23120175 — Huỳnh Thái Toàn

Total we claim: 95 / 100

The final archive must match the reviewed merged `pa3` revision and its seven
successful tag jobs. [PA3.md](PA3.md) explains that verification; the archive
verification sidecar records the actual run and SHA. No coursework upload is performed.

| Criterion                       | Max | We claim | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ------------------------------- | --: | -------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| End-to-end deployed flow        |  15 |       15 | Public HTTPS and five synthetic accounts in [`PA3.md`](PA3.md). The [final 2:06.60 recording](docs/evidence/pa3-review/live-model/README.md) authors the exact official sort-merge lab and all five tests through the product, then submits s01/s04/s09: all AC, live semantic pass/violation/violation, original-line evidence, saved TA confirmation after reload, student privacy, cross-class 404 and blocked publication. Initial [public recovery evidence](docs/evidence/pa3-azure/README.md) covers stop/submit/restart. Deployment began before the updated November 29 deadline.                                                                                                         |
| Queue and sandbox               |  20 |       20 | Separate leased worker with attempt fences and bounded retries; `TestStopSubmitRestartStoresExactlyOneVerdictSet` and `TestTwoWorkersJudgeOneSubmissionExactlyOnce` ([flow tests](apps/backend/internal/flow/flow_test.go)). All nine real container attacks passed without skips on the actual Azure host ([log](docs/evidence/pa3-azure/sandbox.txt)); public worker stop/submit/restart and durable verdict checks are recorded. The `pa3` tag triggers the additive Sandbox attacks job; verify the [matching SHA/run](https://github.com/JakeConal/leetkob/actions/workflows/quality.yml?query=branch%3Apa3) with the archive tag.                                                            |
| Hard rules first                |  15 |       14 | Syntax-aware libclang and [trap/indirection tests](apps/backend/internal/checker/hardrule). Model-off team cases matched 10/10; the [full official offline run](docs/evidence/pa3-review/model-off/) decided all 55 ban-list pairs with zero wrongful deductions. The [live prompts](docs/evidence/pa3-review/live-model/sanitized-calls.json) request only C2; C1 never dispatches a model call. Deducted 1 because unsupported C++ analysis still falls back to unsure; observed coverage does not prove every possible construct.                                                                                                                                                               |
| Semantic model                  |  20 |       18 | [First real full dev run](evaluation/dev-run/README.md): 102 predictions, actual request IDs/tokens/time/cost and unmodified scorer report. [Deployed NIM proof](docs/evidence/pa3-review/live-model/README.md) adds three actual calls and exact sanitized prompts: 2,820 input + 481 output tokens, USD 0.00 configured developer-tier cost, no repair/failure. Comments/identity are removed, original-line evidence and strict schemas are validated, no model deduction is accepted, and the 80-submission arithmetic is documented. Deducted 2 for the first run's missed violations: recall remains 78.4%, below the later PA#4 target, though PA#3 has no overall 90% threshold.           |
| Initial limits and review queue |  10 |        9 | [Semantic tests](apps/backend/internal/semantic) cover invalid schema/evidence, timeout, conservative USD 0.25 reservations, retries and concurrency without network. [PostgreSQL budget tests](apps/backend/internal/adapters/postgres/budget_test.go) verify persistent free-call quotas. [Review contracts](apps/backend/internal/httpapi/reviews_test.go) cover private allowlists, stale decisions and no publication bypass; [live runtime readback](docs/evidence/pa3-review/live-model/runtime.txt) confirms three settled calls and the durable cap. Deducted 1 because live timeout/exhaustion are not deliberately induced; those failure paths are verified deterministically instead. |
| Checker and first eval          |  10 |       10 | Dataset-independent command and Dockerfile with `MODEL_OFF=1` before provider setup, run network-none with memory/PID limits ([`apps/backend/checker/README.md`](apps/backend/checker/README.md)); image test `TestImageMatchesTeamCaseLabels` matched all ten team-case predictions with zero calls. Five labelled team cases include trap and injection tags ([`evaluation/team-cases`](evaluation/team-cases)). All 128 official files are included unchanged under the lecturer’s course hand-in instruction and verified at the start of npm test. A full model-off run produced 102 pairs: 55 ban-list decisions, 47 semantic unsure, zero wrongful deductions and zero model calls.         |
| Tests first and focused history |   5 |        4 | Implementation landed in focused commits with tests before or beside the code, and AI-LOG records deliberate faults that failed the intended test and were restored (D1-D14, [`AI-LOG.md`](AI-LOG.md)). Deducted 1 because the two parallel implementations and the later adoption merge (D14) make the history harder to read than a single focused line.                                                                                                                                                                                                                                                                                                                                         |
| AI log                          |   5 |        5 | [`AI-LOG.md`](AI-LOG.md) records tools and concrete changed/rejected decisions for every part (D13 approval, D14 adopting `main` and porting only unique evidence), including the fixes found during the real run and the refusal to invent deployment or tag evidence.                                                                                                                                                                                                                                                                                                                                                                                                                            |

| **Total** | 100 | **95** | Evidence map: [`PA3.md`](PA3.md). |
| --------- | --: | -----: | --------------------------------- |

## What we did not manage

- **Future availability remains an obligation.** The public service began on
  November 28, before the updated November 29, 22:00 Asia/Saigon deadline.
  The final live-model walkthrough is verified, but one snapshot cannot
  guarantee service retention until PA#4 marks or the owner's credit expiry.
- **Scanner findings remain open.** PR #50 requests Toanapr and XuXinhNo1.
  SonarCloud flags Azure identity/encryption configuration for review; Azure
  confirms platform-managed disk encryption, public disk export is disabled,
  and the VM deliberately has no Azure identity. No rule or check was disabled,
  no green Sonar result is claimed. The security command passes its documented
  high-severity threshold but npm audit reports nine moderate development-tool
  dependency advisories. No check, threshold or dependency was changed to hide them.
  An agent review is not teammate approval.
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
No label or threshold changed. Separately authorized deployment verification
made exactly three actual NIM requests, recorded in
[`live-model/`](docs/evidence/pa3-review/live-model/README.md); it does not replace
the first full evaluation.
The actual-host qualification and public reports are linked from
[`docs/evidence/pa3-azure/README.md`](docs/evidence/pa3-azure/README.md).

The earlier `_91.zip` artifact represents the initial release and is superseded
by this review. The final `_95.zip` is generated only after normal protected merge
and seven successful matching tag jobs, from a clean clone with a real `.git`
directory. Its sidecar records clean extraction, Git integrity, official-byte
verification, SHA-256 and exact tag CI. Match that CI head to
`git rev-parse pa3^{commit}`. Coursework upload is not performed.
