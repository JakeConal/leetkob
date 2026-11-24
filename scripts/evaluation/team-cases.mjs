/* global URL */
// Validation helpers for the team-authored evaluation cases.
// Compiles each solution with the fixed C++17 settings and runs the case
// tests, comparing output tokens. No network access.

import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export const teamCasesRoot = new URL("../../evaluation/team-cases", import.meta.url).pathname;

export function listProblems(root = teamCasesRoot) {
  return readdirSync(join(root, "problems")).sort();
}

export function listSolutions(problem, root = teamCasesRoot) {
  return readdirSync(join(root, "problems", problem, "solutions"))
    .filter((name) => /\.(cpp|cc|cxx)$/.test(name))
    .sort();
}

export function readConstraints(problem, root = teamCasesRoot) {
  const raw = JSON.parse(readFileSync(join(root, "problems", problem, "constraints.json"), "utf8"));
  return Array.isArray(raw) ? raw : (raw.constraints ?? []);
}

export function readLabels(root = teamCasesRoot) {
  return readFileSync(join(root, "labels", "labels.jsonl"), "utf8")
    .split("\n")
    .filter((line) => line.trim() !== "")
    .map((line) => JSON.parse(line));
}

function tokens(text) {
  return text.split(/\s+/).filter(Boolean);
}

export function runCase(problem, solution, root = teamCasesRoot) {
  const workDir = mkdtempSync(join(tmpdir(), "team-case-"));
  try {
    const source = join(root, "problems", problem, "solutions", solution);
    const binary = join(workDir, "program");
    execFileSync("g++", ["-std=c++17", "-O2", "-o", binary, source], { stdio: "pipe" });
    const testsDir = join(root, "problems", problem, "tests");
    const inputs = readdirSync(testsDir)
      .filter((name) => name.endsWith(".in"))
      .sort();
    const results = [];
    for (const inputName of inputs) {
      const base = inputName.slice(0, -3);
      const input = readFileSync(join(testsDir, inputName), "utf8");
      const expected = readFileSync(join(testsDir, `${base}.out`), "utf8");
      const started = Date.now();
      const stdout = execFileSync(binary, [], { input, encoding: "utf8", timeout: 3000 });
      results.push({
        test: base,
        passed: tokens(stdout).join(" ") === tokens(expected).join(" "),
        elapsedMs: Date.now() - started,
      });
    }
    return { problem, solution, results, accepted: results.every((test) => test.passed) };
  } finally {
    rmSync(workDir, { recursive: true, force: true });
  }
}
