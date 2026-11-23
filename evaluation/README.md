# Evaluation source baseline (task 1.4)

The official dev dataset and scorer are pinned together at
[`552c811db2d35a23d641bc4fdffc1ac120be1a28`](https://github.com/fithcmus/awad-leetkob-eval/tree/552c811db2d35a23d641bc4fdffc1ac120be1a28).
`official-source.json` records SHA-256 over the original bytes of all 128 tracked
files, including the README, scorer, labels, statements, constraints, tests and
solutions. Git commit identity and individual content hashes serve different
purposes; a README blob hash alone does not pin the dataset.

Provenance: the lecturer's [evaluation repository](https://github.com/fithcmus/awad-leetkob-eval)
is referenced by the [reconciled class requirements](../docs/requirements/acceptance-matrix.md).
At this revision it has 6 problems, 47 solutions and 102 labelled pairs (65 pass,
37 violation), as documented by upstream. The pinned tree has no license file,
SPDX declaration, or explicit reuse grant. Do not infer an open-source license
from public access. Fetch locally for course evaluation; obtain explicit upstream
permission before redistributing or relicensing its contents. No upstream source
or label content is vendored here.

## Fetch and verify

Requires Git and Node >=22.13, with no added packages. Run from the repository root.
Choose new directories outside the checkout; do not reuse a candidate workspace.

```bash
git clone https://github.com/fithcmus/awad-leetkob-eval.git /tmp/leetkob-official
git -C /tmp/leetkob-official checkout --detach 552c811db2d35a23d641bc4fdffc1ac120be1a28
node scripts/evaluation/sources.mjs verify /tmp/leetkob-official
node scripts/evaluation/sources.mjs stage /tmp/leetkob-official /tmp/leetkob-prediction-inputs
```

Verification rejects changed bytes, missing or extra files, and symbolic links.
Only `.git` metadata is excluded from inventory verification. The upstream scorer
must pass verification before and after scoring. Never format, patch, or wrap its
contents to change evaluation outcomes. Run it directly in the trusted scoring
step:

```bash
node /tmp/leetkob-official/eval.mjs --data /tmp/leetkob-official --predictions /tmp/predictions.jsonl --json /tmp/summary.json
node scripts/evaluation/sources.mjs verify /tmp/leetkob-official
```

Treat scorer exits 1 (threshold failure) and 2 (invalid inputs) as failures. The
fixed thresholds are zero wrongful deductions, recall >=90% with labelled original
line evidence, and unsure <=20%. Do not derive predictions from ground truth.

## Prediction and scoring separation

The trusted preparer copies only these paths, preserving bytes and identifiers:

```text
prediction-inputs/
  problems/<problem-id>/statement.md
  problems/<problem-id>/constraints.json
  solutions/<problem-id>/sNN.cpp
```

The staging command does not copy `labels.jsonl`, tags, tests, scorer, README,
Git metadata, or other files. A fresh destination is mandatory; stale directories
and all input symlinks are rejected. Problem IDs are discovered, not hard-coded.

Run the future checker as `checker --data <prediction-inputs> --predictions
<output.jsonl>` inside the separately qualified candidate execution boundary.
Mount only staged inputs and a prediction output location. Keep the original
source tree, labels, source manifest, team review records and trusted scorer
outside that boundary. Directory separation by itself does not stop a process
from reading the host: do not run untrusted prediction code directly on the
trusted host. Prediction generation receives no labels; the subsequent trusted
scoring process receives labels and the prediction artifact. Never pass provider
credentials to candidate code, including environment variables.

This phase-0 deliverable verifies the input artifact boundary and scorer byte
integrity. It does not implement or qualify the Go checker (5.1), scorer runner
(5.7), credential-free trusted broker/gate (5.8/7.6), or run live models (5.9).
Recheck the boundary through those real entry points when they exist. No checker
quality or production isolation is claimed by the deterministic staging tests.

## Separate team dataset layout

Keep reviewed team cases in a distinct directory, never overlay the official set:

```text
team-dataset/
  problems/<new-problem-id>/statement.md
  problems/<new-problem-id>/constraints.json
  problems/<new-problem-id>/tests/01.in
  problems/<new-problem-id>/tests/01.out
  solutions/<new-problem-id>/s01.cpp
  labels.jsonl
team-review/                         # trusted, not staged for prediction
  provenance.json
  accepted-validation.json
  label-review.json
```

Use the upstream schemas: constraints carry id/text/deduction; labels carry
solution (`problem-id/sNN`), constraint, pass|violation label, evidence line numbers
and upstream-supported tags. Predictions use pass|violation|unsure and original
line/text evidence. Use lowercase hyphenated problem IDs and `sNN.cpp` filenames.
Provenance records authorship, reuse permission/license, anonymization, source
revision and per-file SHA-256. Accepted validation records C++17 compilation
(`g++ -std=c++17 -O2`), all test input/output hashes and token comparison results.
Independent human label review records reviewer, reason, evidence, revision and
date before checker predictions are inspected. Do not change labels to improve
scores. Task 5.7 supplies and reviews actual team cases; no synthetic placeholder
is presented as an approved team dataset here.

## Requirement-to-test design

| Requirement / risk                                | Input or state                                                     | Independent expected result / defect detected                                                       |
| ------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| No labels or scoring tags reach prediction inputs | Unfamiliar problem plus sentinel labels/scorer/test output         | Exactly three allowed input paths, original bytes; detects recursive copying or allowlist expansion |
| No stale labels or link escape                    | Existing destination, nested destination, source symlink to labels | Preparation rejects; detects reuse and dereference leaks                                            |
| Pinned upstream remains unchanged                 | Missing file, extra file, changed bytes                            | Verification rejects; detects skipped inventory or hash validation                                  |

Run `node --test scripts/evaluation/sources.test.mjs` (also included in `npm test`).
Verify the real pinned checkout with the command above. These are deterministic,
free checks, with no provider access and no manipulation of upstream ground truth.

## PA#3 reconciliation: in-repository bytes vs the no-license source

The PA#3 rubric expects the published dev-set bytes to be preserved in the
submitted repository. The pinned upstream tree has **no redistribution
license**, so vendoring its bytes would violate the provenance policy above.
The resolution recorded here (and in AI-LOG D13 / the acceptance matrix) is:

- the repository commits the byte-hash manifest (`official-source.json`), the
  verifier/stager, and the team's committed prediction run referenced by the
  manifest revision, problem IDs and input hashes;
- the marker re-fetches the pinned commit with `git clone`/`checkout` and
  verifies all 128 files differ by zero bytes before and after scoring;
- no upstream bytes are redistributed by this repository. If the lecturer
  requires literal in-repository bytes, that requires explicit upstream
  permission or a marker-supplied dataset; the policy is not silently changed.

Verification evidence (2026-11-23): cloning the pinned commit and running
`verifyOfficial` reported **128 files verified byte-identically**, matching the
manifest.

## Team cases (task 7.3)

The reviewed team dataset lives in `evaluation/team-cases/` with the layout the
checker discovers (`problems/<id>/{statement.md,constraints.json,solutions/*.cpp}`)
plus `tests/` for Accepted validation and `labels/labels.jsonl` for the
independent labels. Five cases, ten solutions, tags covering traps (student
function named `sort`, correct-output violations) and injection (prompt text in
comments and a string literal). Labels were written by reading each solution
before any tool ran; they cover only deterministic ban-list constraints.

Commands:

```bash
node --test scripts/evaluation/team-cases.test.mjs   # layout, labels, Accepted validation
```

That test compiles every solution with `g++ -std=c++17 -O2` and compares output
tokens against the case tests. On 2026-11-23 the checker image with
`MODEL_OFF=1` under `--network none` produced 10 predictions for these cases and
matched the independent labels **exactly (0 mismatches, 0 model calls)**, so the
labels are not tuned to predictions.
