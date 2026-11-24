import assert from "node:assert/strict";
import test from "node:test";
import { spawnSync } from "node:child_process";

import {
  listProblems,
  listSolutions,
  readConstraints,
  readLabels,
  runCase,
  teamCasesRoot,
} from "./team-cases.mjs";
import { existsSync } from "node:fs";

const hasCompiler = spawnSync("g++", ["--version"], { stdio: "ignore" }).status === 0;

test("team case layout is complete", () => {
  assert.ok(existsSync(teamCasesRoot), `missing ${teamCasesRoot}`);
  const problems = listProblems();
  assert.ok(problems.length >= 5, `expected at least 5 team cases, found ${problems.length}`);
  for (const problem of problems) {
    const solutions = listSolutions(problem);
    assert.ok(solutions.length >= 2, `${problem} needs at least 2 solutions`);
    assert.ok(readConstraints(problem).length >= 1, `${problem} needs a constraint`);
  }
});

test("labels cover every solution/constraint pair exactly once", () => {
  const labels = readLabels();
  const seen = new Map();
  for (const label of labels) {
    const key = `${label.solution}::${label.constraint}`;
    assert.equal(seen.has(key), false, `duplicate label for ${key}`);
    assert.ok(["pass", "violation", "unsure"].includes(label.verdict), `bad verdict in ${key}`);
    assert.ok(
      typeof label.rationale === "string" && label.rationale.length > 0,
      `missing rationale in ${key}`,
    );
    seen.set(key, true);
  }
  for (const problem of listProblems()) {
    for (const solution of listSolutions(problem)) {
      for (const constraint of readConstraints(problem)) {
        const key = `${problem}/${solution.replace(/\.(cpp|cc|cxx)$/, "")}::${constraint.id}`;
        assert.ok(seen.has(key), `missing label for ${key}`);
      }
    }
  }
});

test("every team case solution is Accepted", { skip: !hasCompiler && "g++ is unavailable" }, () => {
  for (const problem of listProblems()) {
    for (const solution of listSolutions(problem)) {
      const result = runCase(problem, solution);
      assert.equal(
        result.accepted,
        true,
        `${problem}/${solution} failed: ${JSON.stringify(result.results)}`,
      );
    }
  }
});
