# PA#3 public walkthrough

The application is deployed at
<https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>.
First public HTTPS verification was on November 28, 2026, before the updated
November 29, 22:00 Asia/Saigon submission deadline. Retention until PA#4 marks
remains an ongoing obligation.

The final **2:06.60 uncut fallback recording** uses the real deployed NVIDIA NIM
worker and the three official sort-merge cases:
[watch/download](pa3-review/live-model/walkthrough.webm).
Its [evidence index](pa3-review/live-model/README.md) includes actual provider IDs,
sanitized requests, token/cost records, screenshots and runtime budget readback.
Earlier HTTP privacy, worker-recovery, direct PostgreSQL verdict counts and
actual-host sandbox qualification remain in
[`pa3-azure/README.md`](pa3-azure/README.md), explicitly identified as initial evidence.

Sign-in credentials for the five intentionally public synthetic accounts are in
[`../../PA3.md`](../../PA3.md). The recording creates a new lab through the UI,
submits official compliant, forbidden-sort and injection-comment solutions,
records an evidence-backed TA decision, verifies it after reload and demonstrates
cross-class denial. No marking lab is pre-seeded and no marks are published.
All three reach AC; live semantic results are pass, violation and violation.
The separate first full evaluation remains at its original measured metrics.

The current runtime supersedes the November 24 local-only staging configuration.
In particular, the trusted worker contains the Docker client and no native parser;
libclang analysis runs in its own qualified isolated container. The historical
choice to skip hosting was superseded by the owner's authorization to use Toàn's
student credits. No uptime, recording, CI result or provider request is backdated.
