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
from public access. The lecturer explicitly requires unchanged dev-set bytes
in the course submission; all 128 files are included in `official/` for that
purpose. [Provenance](official-provenance.md) records the instruction without
claiming a general open-source redistribution license.

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

Run the checker as `checker --data <prediction-inputs> --predictions
<output.jsonl>` inside the separately qualified candidate execution boundary.
Mount only staged inputs and a prediction output location. Keep the original
source tree, labels, source manifest, team review records and trusted scorer
outside that boundary. Directory separation by itself does not stop a process
from reading the host: do not run untrusted prediction code directly on the
trusted host. Prediction generation receives no labels; the subsequent trusted
scoring process receives labels and the prediction artifact. Never pass provider
credentials to candidate code, including environment variables.

The qualified checker image and execution commands are documented in
[`apps/backend/checker/README.md`](../apps/backend/checker/README.md). The committed
real first run is in [`dev-run/`](dev-run/README.md). The input preparation tests
verify separation and integrity; the image and actual-host tests verify the
execution boundary independently.

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
scores. The actual team cases and their provenance are in `team-cases/`.

## Requirement-to-test design

| Requirement / risk                                | Input or state                                                     | Independent expected result / defect detected                                                       |
| ------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| No labels or scoring tags reach prediction inputs | Unfamiliar problem plus sentinel labels/scorer/test output         | Exactly three allowed input paths, original bytes; detects recursive copying or allowlist expansion |
| No stale labels or link escape                    | Existing destination, nested destination, source symlink to labels | Preparation rejects; detects reuse and dereference leaks                                            |
| Pinned upstream remains unchanged                 | Missing file, extra file, changed bytes                            | Verification rejects; detects skipped inventory or hash validation                                  |

Run `node --test scripts/evaluation/sources.test.mjs` (also included in `npm test`).
Verify the real pinned checkout with the command above. These are deterministic,
free checks, with no provider access and no manipulation of upstream ground truth.

## Official bytes in the submission

`evaluation/official/` contains the exact pinned published tree. The root
`npm test` first verifies all 128 files against `official-source.json`.
`.gitattributes` disables line-ending rewriting for those files; lint and format
ignore only that immutable directory. Never edit labels, tests, scorer or source
to improve the score. The course hand-in instruction and license limits are in
[`official-provenance.md`](official-provenance.md).

## Team cases (task 7.3)

`evaluation/team-cases/` holds five team-authored cases (ten solutions) in
the layout the checker discovers, plus `tests/` for Accepted validation and
`labels/labels.jsonl` for independent labels. Tags cover traps (a student
function named `sort`, correct-output violations) and injection (prompt text
in comments and a string literal). Labels were written by reading each
solution before running any tool and cover only deterministic ban-list pairs.

```bash
node --test scripts/evaluation/team-cases.test.mjs
```

The test compiles every solution with `g++ -std=c++17 -O2` and compares output
tokens. On 2026-11-24 the checker image under `--network none` with
`MODEL_OFF=1` produced 10 predictions that matched the labels exactly, so the
labels are not tuned to predictions. The image test in
`apps/backend/internal/checker/image_test.go` repeats this comparison.
