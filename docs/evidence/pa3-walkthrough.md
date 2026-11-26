# PA#3 rubric walkthrough — configuration, evidence and gaps

Recorded 2026-11-24 on `codex/port-pa3-unique-evidence` (based on `main`
after PRs #41/#42). No public deployment exists: the owner chose to skip
external provisioning, so this document records the local staging run and
the CI evidence instead of a live URL.

## Configuration exercised

| Element    | Configuration                                                                                                                         |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Database   | PostgreSQL 18.7, `postgres@sha256:eb63372c…`, disposable local instance                                                               |
| API        | `cmd/api`, additive migrations at startup, `/healthz`, HttpOnly session cookies                                                       |
| Worker     | `cmd/worker` with judge + check loops, `MODEL_OFF=1` (online semantic checks are offline/unsure)                                      |
| Sandbox    | `leetkob-cpp17:pa3` built from `apps/backend/sandbox/cpp17`, all nine controls, live attacks                                          |
| Hard rules | libclang `cpp-analyzer` (bundled in the worker image, `CHECKER_ANALYZER`)                                                             |
| Web        | Next.js `/live/**` with `/v2` rewrites to the API through the same origin                                                             |
| Checker    | `leetkob-checker:pa3` digest-pinned image with analyzer; `MODEL_OFF=1` for marking, real model only in the separately authorized eval |

## What actually ran

1. **CI on the submitted revision** — run
   [45306355723](https://github.com/JakeConal/leetkob/actions/runs/45306355723)
   (PR #44) completed green across Lint, Format, Typecheck, Build, Test and
   Security. The Test job executed the merged pipeline, including
   `TestMergedPipeline` (store → sandbox → hard rule → semantic),
   `TestHTTPIngressToJudgedResultStaysOwnerPrivate` (ingress → judged →
   owner/assigned-TA reads; other student and other-class TA get the
   concealed 404), `TestStopSubmitRestartStoresExactlyOneVerdictSet`,
   `TestTwoWorkersJudgeOneSubmissionExactlyOnce`, the sandbox attack suite
   and the checker image test.
2. **Local staging** — `docker compose` brought up PostgreSQL, the API and
   the web app; `/healthz` returned `{"status":"ok"}`, `/live` returned 200,
   `cmd/provision` created the five synthetic accounts, and signing in
   through the web's same-origin `/v2` rewrite returned 200 with a session
   cookie. Re-provisioning with different passwords reported no changes and
   the original password still worked (idempotent, never silently reset).
3. **Real-model checker run (authorized)** — the checker image ran on the
   pinned official dev set with NVIDIA NIM `minimaxai/minimax-m3.1-flash`,
   producing the committed evidence in `evaluation/dev-run/`: 102
   predictions, **0 wrongful deductions**, 29/37 violations caught, **78.4 %
   recall**, 3.9 % unsure, unmodified scorer exit 1 (valid PA#3 outcome; no
   PA#3 pass threshold exists).

## Gaps, stated plainly

- **No public deployment.** Per the owner's decision, no hosting was
  provisioned, so the rubric's live-flow criterion is not met. `PA3.md`
  therefore links this document and CI instead of a URL, claims no uptime
  and no fallback recording. `deploy/` contains the prepared, locally
  verified artifacts and a $0 free-VM proposal.
- **No fallback recording.** The at-most-five-minute walkthrough was not
  recorded; the local staging evidence above is the substitute, and the
  submission does not claim a recording exists.
- **Online semantic checks in the deployed worker are offline.** The worker
  runs the semantic checker against a deterministic offline model, so online
  semantic findings are `unsure`; the real-model path exists in the checker
  evaluation (committed above), not in the worker.
- **No tag pushed at the time of writing.** The `pa3` tag job was added to CI
  but has not run on a real tag, so no tag-run URL is claimed; `PA3.md` links
  the filtered tag run list, which becomes valid only after the tag is pushed.
