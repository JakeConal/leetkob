# PA#3 public walkthrough

The application is deployed at
<https://leetkob-pa3-21ae08a0.southeastasia.cloudapp.azure.com/live>.
First public HTTPS verification was on November 28, 2026, after the November 25
submission deadline. Availability since the deadline is not met.

The actual **2:06.68 fallback recording**, browser screenshots, HTTP privacy and
worker-recovery checks, direct PostgreSQL verdict count, actual-host sandbox
qualification and runtime health are indexed in
[`pa3-azure/README.md`](pa3-azure/README.md).
[Watch/download the walkthrough](pa3-azure/walkthrough.webm).

Sign-in credentials for the five intentionally public synthetic accounts are in
[`../../PA3.md`](../../PA3.md). The recording creates a new lab through the UI,
submits compliant and forbidden-sort solutions, records an evidence-backed TA
decision and demonstrates cross-class denial. No marking lab is pre-seeded and
no marks are published. Semantic findings remain `unsure` in the online worker;
the committed separately authorized real-model evaluation is linked from the index.

The current runtime supersedes the November 24 local-only staging configuration.
In particular, the trusted worker contains the Docker client and no native parser;
libclang analysis runs in its own qualified isolated container. The historical
choice to skip hosting was superseded by the owner's authorization to use Toàn's
student credits. No uptime, recording, CI result or provider request is backdated.
