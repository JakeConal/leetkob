import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { officialSource, stagePredictionInputs, verifyOfficial } from "./sources.mjs";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "leetkob-source-test-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const data = join(root, "data");
  mkdirSync(join(data, "problems", "unseen-id", "tests"), { recursive: true });
  mkdirSync(join(data, "solutions", "unseen-id"), { recursive: true });
  writeFileSync(join(data, "problems/unseen-id/statement.md"), "Return 1.");
  writeFileSync(
    join(data, "problems/unseen-id/constraints.json"),
    '[{"id":"C1","text":"Use recursion","deduction":3}]',
  );
  writeFileSync(join(data, "solutions/unseen-id/s01.cpp"), "int main() { return 0; }\n");
  writeFileSync(join(data, "labels.jsonl"), "SECRET_LABEL_AND_TAG");
  writeFileSync(join(data, "eval.mjs"), "SECRET_SCORER");
  writeFileSync(join(data, "problems/unseen-id/tests/01.out"), "SECRET_TEST");
  return { root, data, output: join(root, "inputs") };
}

test("prediction input staging preserves unfamiliar IDs and original bytes, excludes all scoring data", (t) => {
  const { data, output } = fixture(t);
  const paths = stagePredictionInputs(data, output);
  assert.deepEqual(paths.sort(), [
    "problems/unseen-id/constraints.json",
    "problems/unseen-id/statement.md",
    "solutions/unseen-id/s01.cpp",
  ]);
  assert.deepEqual(readdirSync(output).sort(), ["problems", "solutions"]);
  assert.deepEqual(readdirSync(join(output, "problems/unseen-id")).sort(), [
    "constraints.json",
    "statement.md",
  ]);
  for (const path of paths) {
    assert.deepEqual(readFileSync(join(output, path)), readFileSync(join(data, path)));
    assert.doesNotMatch(readFileSync(join(output, path), "utf8"), /SECRET_/);
  }
});

test("staging rejects stale destination, nesting, and symlink label smuggling", (t) => {
  const { data, output } = fixture(t);
  mkdirSync(output);
  writeFileSync(join(output, "labels.jsonl"), "stale");
  assert.throws(() => stagePredictionInputs(data, output), /EEXIST/);
  assert.throws(() => stagePredictionInputs(data, join(data, "inputs")), /outside/);
  symlinkSync(join(data, "labels.jsonl"), join(data, "solutions/unseen-id/s02.cpp"));
  assert.throws(() => stagePredictionInputs(data, `${output}-new`), /Symlink rejected/);
});

test("official verifier rejects missing, extra, and changed scorer files", (t) => {
  const { root } = fixture(t);
  const source = join(root, "official");
  mkdirSync(source);
  assert.throws(() => verifyOfficial(source), /inventory/);
  // Reproduce the pinned inventory without copying official content into tests.
  for (const path of Object.keys(officialSource.sha256)) {
    mkdirSync(join(source, path, ".."), { recursive: true });
    writeFileSync(join(source, path), "changed");
  }
  assert.throws(() => verifyOfficial(source), /SHA-256 mismatch/);
  writeFileSync(join(source, "unexpected"), "extra");
  assert.throws(() => verifyOfficial(source), /inventory/);
});
