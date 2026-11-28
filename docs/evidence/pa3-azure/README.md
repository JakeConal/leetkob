# PA#3 public Azure release evidence

This directory preserves the **initial model-off deployment**. The final
official three-case recording with actual deployed NIM calls is in
[`../pa3-review/live-model/`](../pa3-review/live-model/README.md).

Public service: <https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>.
Five synthetic sign-ins are listed in [`../../../PA3.md`](../../../PA3.md).
First host start: `2026-11-28T01:57:20Z`; first observed valid public HTTPS:
`2026-11-28T01:57:30Z`. This precedes the updated November 29, 22:00 Asia/Saigon deadline; no earlier
availability is claimed. The owner confirmed Toàn's student-credit coverage.

## Recording and browser checks

[Watch/download the actual walkthrough](walkthrough.webm): **2:06.68**, VP9 WebM,
1440×900, silent. This records interaction with the real public HTTPS service;
it is not a local mockup. The original browser capture is preserved without cuts.
The source/report clock timestamps and encoded media duration are recorded
separately; the five-minute limit is checked against the actual media duration.

The recording shows:

1. Assigned TA signs in and creates a lab through the UI with two constraints,
   a sample and hidden test, execution limits and an open submission window.
2. Student A1 submits merge sort; actual isolated judging returns AC.
3. Student A2 submits a forbidden library sort; judging also returns AC.
4. The assigned TA sees original-line `std::sort` evidence, saves a
   revision-bound violation decision and leaves semantic findings `unsure`.
   The UI says no marks were published and publication remains blocked.
5. The other-class TA signs in and receives a concealed 404 for that review queue.

[`browser-report.json`](browser-report.json) records actual lab/submission IDs,
viewports and errors. Both 1440×900 and 390×844 were checked, with no page errors,
unexpected console errors or document overflow on mobile. The browser ran in the
official digest-pinned Playwright 1.65.1 image after native Chromium could not
start with the host's missing libraries. The first automation attempt targeted
an input covered by its visible custom label and timed out; the successful run
clicks the real label and asserts the radio selection. No application code or
assertion was weakened to record it.

Screenshots: [created lab](lab-created.png), [desktop reviews](desktop-reviews.png),
[mobile reviews](mobile-reviews.png), [cross-class denial](cross-class-denied.png).

## HTTP, recovery and isolation

| Artifact                                     | Actual observation                                                                                                                                                                                                                                                        |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [http-report.json](http-report.json)         | Public AC/pass, AC/violation with evidence, injection AC/violation, WA and CE checker bypass; sample-only tests; student-safe fields; other student and other-class TA receive 404. A submission is accepted queued while the worker is stopped and judged after restart. |
| [restart-verdict.json](restart-verdict.json) | Direct PostgreSQL query for the recovery submission: judged, exactly one verdict set, three test results and one judge job.                                                                                                                                               |
| [admin-session.json](admin-session.json)     | The fifth synthetic account signs in and reads its own session; HTTPS cookie is Secure/HttpOnly and CSRF token exists. No session value is recorded.                                                                                                                      |
| [runtime-health.json](runtime-health.json)   | Actual API/worker/web/PostgreSQL health and running Caddy; only Caddy publishes internet-facing ports.                                                                                                                                                                    |
| [sandbox.txt](sandbox.txt)                   | All actual-host sandbox tests, including nine live attacks and disabled-control negative tests, passed without skips.                                                                                                                                                     |
| [checker-image.txt](checker-image.txt)       | Actual-host checker image matches all ten independently labelled team-case predictions with model requests off.                                                                                                                                                           |
| [host.json](host.json)                       | Qualified source revision, Docker version, kernel and architecture.                                                                                                                                                                                                       |

The qualified application/image source is commit
`36e3caf92c8c5a3310dd0593610bd401aacc33b8`. This records the initial deployment. PR #50 subsequently adds the real-model
worker, stricter isolation and byte-identical official inputs. Its live activation
is now independently verified in the final evidence linked above; this original
recording does not prove those later changes. The original working checkout is preserved.
No real marking lab is pre-seeded and no marks are published.

## Evaluation and limits

At initial deployment the public worker ran `MODEL_OFF=1` with isolated libclang analysis. Its semantic
findings remain `unsure`; no provider key is installed and no new model call was
made for that initial release. The separate authorized real-model evaluation is committed
in [`../../../evaluation/dev-run/`](../../../evaluation/dev-run/README.md).
November 28 rescoring with the unchanged pinned official scorer reproduced the
committed JSON summary exactly: zero wrongful deductions, 78.4% recall, 3.9%
unsure, exit 1. All 128 official files verified before and after the rescore.

The VM consumes confirmed student credits. Estimated baseline is USD 42.91/month
before variable disk operations and egress. Azure was re-read after provisioning:
spending limit On, USD 50 monthly budget, all actual and forecast alerts enabled.
Notifications are not an automatic USD 50 cutoff. See
[`../../../deploy/AZURE.md`](../../../deploy/AZURE.md) for recovery and the open
SonarCloud identity/encryption review. The disk uses platform-managed encryption;
public disk export is disabled. Future uptime is not guaranteed by these snapshots.
