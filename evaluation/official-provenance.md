# Official course evaluation inputs

`official/` preserves all 128 files from the lecturer's published evaluation
repository at commit `552c811db2d35a23d641bc4fdffc1ac120be1a28`:
<https://github.com/fithcmus/awad-leetkob-eval/tree/552c811db2d35a23d641bc4fdffc1ac120be1a28>.

The PA#3 rubric explicitly requires the dev set inside the submitted repository,
byte for byte as published. This course hand-in copy follows that instruction;
it is not a claim of a general open-source licence or a relicensing of the
lecturer's files. Upstream has no separate licence file. Preserve upstream
attribution and bytes; the project's licence does not grant additional rights
to these third-party materials. Team-authored cases remain in `team-cases/`.

Authoritative rubric (re-read November 28, version modified November 24):
<https://drive.google.com/file/d/1EGXZQNjbevJ5cE18rgXEujeKJ1hFeFvf/view>.
This supersedes the earlier local decision to omit the required course inputs.

Verify before and after scoring:

```bash
node scripts/evaluation/sources.mjs verify evaluation/official
node evaluation/official/eval.mjs --data evaluation/official \
  --predictions evaluation/dev-run/predictions.jsonl --json /tmp/pa3-rescore.json
node scripts/evaluation/sources.mjs verify evaluation/official
```

Scorer exit 1 remains the honest first-run threshold failure (78.4% recall),
not invalid-input exit 2. No solution, label, expected result or scorer was edited.
Do not compile or parse the C++ inputs outside the qualified isolation boundary.
