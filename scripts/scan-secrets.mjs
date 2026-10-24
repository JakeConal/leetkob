/* global process */

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const rules = [
  ["AWS access key", /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/g],
  ["GitHub token", /\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,})\b/g],
  ["Google API key", /\bAIza[0-9A-Za-z_-]{35}\b/g],
  ["model provider key", /\bsk-[A-Za-z0-9_-]{20,}\b/g],
  ["private key material", /-----BEGIN [A-Z ]*PRIVATE KEY-----/g],
];

const maximumBytes = 2 * 1024 * 1024;
const scannerPath = "scripts/scan-secrets.mjs";

function runGit(args, options = {}) {
  return execFileSync("git", args, {
    cwd: repositoryRoot,
    encoding: options.binary === true ? null : "utf8",
    maxBuffer: 16 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function scanBuffer(buffer, label) {
  if (buffer.length > maximumBytes || buffer.includes(0)) return [];
  const text = buffer.toString("utf8");
  const findings = [];

  for (const [name, pattern] of rules) {
    pattern.lastIndex = 0;
    if (pattern.test(text)) findings.push(`${label}: ${name}`);
  }
  return findings;
}

function scanPath(path) {
  const absolute = resolve(path);
  return scanBuffer(readFileSync(absolute), absolute);
}

function scanWorkingTree() {
  const listed = runGit(["ls-files", "-co", "--exclude-standard", "-z"]);
  const paths = listed.split("\0").filter(Boolean);
  const findings = [];

  for (const path of paths) {
    if (path === scannerPath) continue;
    findings.push(...scanPath(resolve(repositoryRoot, path)));
  }
  return findings;
}

function scanHistory() {
  let objects;
  try {
    objects = runGit(["rev-list", "--objects", "--all"]);
  } catch {
    return [];
  }

  const findings = [];
  const seen = new Set();
  for (const line of objects.split(/\r?\n/)) {
    const separator = line.indexOf(" ");
    if (separator < 1) continue;
    const objectId = line.slice(0, separator);
    const path = line.slice(separator + 1);
    if (path === scannerPath || seen.has(objectId)) continue;
    seen.add(objectId);

    let type;
    try {
      type = runGit(["cat-file", "-t", objectId]).trim();
    } catch {
      continue;
    }
    if (type !== "blob") continue;

    const size = Number(runGit(["cat-file", "-s", objectId]).trim());
    if (!Number.isFinite(size) || size > maximumBytes) continue;
    const buffer = runGit(["cat-file", "blob", objectId], { binary: true });
    findings.push(...scanBuffer(buffer, `history:${objectId.slice(0, 12)}:${path}`));
  }
  return findings;
}

const repositoryRoot = execFileSync("git", ["rev-parse", "--show-toplevel"], {
  encoding: "utf8",
}).trim();
const pathIndex = process.argv.indexOf("--path");
const findings =
  pathIndex >= 0
    ? scanPath(process.argv[pathIndex + 1] ?? "")
    : [...scanWorkingTree(), ...scanHistory()];

if (findings.length > 0) {
  process.stderr.write(`Credential scan failed (${findings.length} finding(s)):\n`);
  for (const finding of findings) process.stderr.write(`- ${finding}\n`);
  process.exitCode = 1;
} else {
  process.stdout.write("Credential scan passed: no high-confidence patterns found.\n");
}
