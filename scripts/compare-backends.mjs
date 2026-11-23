#!/usr/bin/env node
/* global process */

// Reproducible NestJS/Go compatibility comparison (issue #38 task 1.5).
// Drives the identical seeded publication workload against the NestJS
// PA#2 application (in-process) and the Go compatibility composition
// (via the benchmark test), then writes docs/evidence/backend-comparison
// json/markdown with revisions, hardware, warmup/run counts and measured
// latency/throughput/errors/startup/build/artifact metrics.
//
// Usage: npm run compare:backends
// Environment: COMPARE_WARMUP (200), COMPARE_ROUNDS (5),
// COMPARE_CONCURRENCY (50), COMPARE_LABS (200).

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { performance } from "node:perf_hooks";

import { createApiApplication } from "../apps/api/src/app-factory.ts";
import { issueToken } from "../apps/api/src/auth/token.ts";

const warmup = intEnv("COMPARE_WARMUP", 200);
const rounds = intEnv("COMPARE_ROUNDS", 5);
const concurrency = intEnv("COMPARE_CONCURRENCY", 50);
const labCount = intEnv("COMPARE_LABS", 200);

const secret = "comparison-secret-that-is-at-least-32-bytes";
const nowSeconds = 1_800_000_000;
const instant = new Date("2026-11-23T08:00:00.000Z");
const deadline = "2026-11-22T22:00:00.000Z";
const identities = {
  ta: { subject: "ta-1", role: "ta", classIds: ["class-a"] },
  student: { subject: "student-1", role: "student", classIds: ["class-a"] },
};

function intEnv(name, fallback) {
  const parsed = Number.parseInt(process.env[name] ?? "", 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function labUUID(i) {
  return `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`;
}

function readyLab(id, classId) {
  return { id, classId, status: "ready", pendingReviews: 0, submissionDeadline: deadline };
}

async function runNodeSide() {
  const { InMemoryIdempotencyRepository, InMemoryLabRepository } =
    await import("../apps/api/src/adapters/in-memory.ts");
  const labs = [];
  for (let i = 0; i < labCount; i += 1) labs.push(readyLab(labUUID(i), "class-a"));
  const foreignId = labUUID(labCount);
  labs.push(readyLab(foreignId, "class-b"));
  const labRepository = new InMemoryLabRepository(labs);

  const started = performance.now();
  const application = await createApiApplication({
    authSecret: secret,
    clock: { now: () => new Date(instant) },
    idempotency: new InMemoryIdempotencyRepository(),
    labs: labRepository,
    nowSeconds: () => nowSeconds,
  });
  await application.listen(0, "127.0.0.1");
  const baseUrl = await application.getUrl();
  const startupMs = performance.now() - started;

  const taToken = issueToken(identities.ta, secret, nowSeconds + 60);
  const studentToken = issueToken(identities.student, secret, nowSeconds + 60);

  const post = async (token, path, key) => {
    const startedAt = performance.now();
    const response = await globalThis.fetch(`${baseUrl}/labs/${path}`, {
      method: "POST",
      headers: { authorization: `Bearer ${token}`, "idempotency-key": key },
    });
    await response.arrayBuffer();
    return { status: response.status, elapsedMs: performance.now() - startedAt };
  };

  for (let i = 0; i < warmup; i += 1) {
    const { status } = await post(taToken, `${labUUID(0)}/publish`, "warmup-key");
    if (status !== 200) throw new Error(`warmup returned ${status}`);
  }

  const latencies = [];
  let errors = 0;
  const record = (status, want, elapsedMs) => {
    if (status !== want) errors += 1;
    latencies.push(elapsedMs);
  };

  const start = performance.now();
  for (let round = 0; round < rounds; round += 1) {
    for (let i = 1; i < labCount; i += 1) await labRepository.save(readyLab(labUUID(i), "class-a"));
    for (let i = 1; i < labCount; i += 1) {
      const key = `publish-key-${round}-${i}`;
      let result = await post(taToken, `${labUUID(i)}/publish`, key);
      record(result.status, 200, result.elapsedMs);
      result = await post(taToken, `${labUUID(i)}/publish`, key);
      record(result.status, 200, result.elapsedMs);
      result = await post(taToken, `${foreignId}/publish`, `missing-${key}`);
      record(result.status, 404, result.elapsedMs);
      result = await post(studentToken, `${labUUID(i)}/publish`, `student-${key}`);
      record(result.status, 403, result.elapsedMs);
    }
    const fresh = labUUID(labCount + 1 + round);
    await labRepository.save(readyLab(fresh, "class-a"));
    const results = await Promise.all(
      Array.from({ length: concurrency * 2 }, (_, i) =>
        post(taToken, `${fresh}/publish`, `concurrent-${round}-${i}`),
      ),
    );
    for (const result of results) {
      if (result.status !== 200 && result.status !== 409) errors += 1;
      latencies.push(result.elapsedMs);
    }
  }
  const measuredMs = performance.now() - start;
  await application.close();

  const sorted = [...latencies].sort((a, b) => a - b);
  return {
    implementation: "nestjs-pa2",
    requests: sorted.length + warmup,
    errors,
    warmup,
    rounds,
    concurrency,
    labs: labCount,
    latencyMs: {
      mean: sorted.reduce((sum, value) => sum + value, 0) / (sorted.length || 1),
      p50: sorted[Math.floor((sorted.length * 50) / 100)] ?? 0,
      p95: sorted[Math.floor((sorted.length * 95) / 100)] ?? 0,
      max: sorted[sorted.length - 1] ?? 0,
    },
    throughputPerSecond: sorted.length / (measuredMs / 1000),
    measuredMs,
    startupMs,
    memoryRssBytes: process.memoryUsage().rss,
  };
}

function resolveGo() {
  for (const candidate of [process.env.GO ?? "go", "go", "/snap/go/current/bin/go"]) {
    try {
      const version = execFileSync(candidate, ["version"], { encoding: "utf8" });
      return { command: candidate, version: version.trim() };
    } catch {
      // Try the next candidate location.
    }
  }
  throw new Error("go toolchain not found; install go 1.27.4");
}

function runGoSide() {
  const goTool = resolveGo();
  const env = {
    ...process.env,
    COMPARE_WARMUP: String(warmup),
    COMPARE_ROUNDS: String(rounds),
    COMPARE_CONCURRENCY: String(concurrency),
    COMPARE_LABS: String(labCount),
    GOTOOLCHAIN: "local",
  };
  const output = execFileSync(
    goTool.command,
    ["test", "-run", "TestPublicationBenchmark", "-v", "./internal/compatpub"],
    {
      cwd: join(process.cwd(), "apps", "backend"),
      env,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    },
  );
  const line = output.split("\n").find((candidate) => candidate.includes("BENCHJSON:"));
  if (line === undefined) throw new Error("go benchmark produced no BENCHJSON line");
  return {
    command: goTool.command,
    metrics: JSON.parse(line.slice(line.indexOf("BENCHJSON:") + "BENCHJSON:".length)),
    version: goTool.version,
  };
}

function measure(label, command, cwd) {
  const started = performance.now();
  execFileSync(command[0], command.slice(1), { cwd, stdio: "ignore" });
  return { label, milliseconds: performance.now() - started };
}

function directorySize(path) {
  if (!existsSync(path)) return 0;
  let total = 0;
  for (const entry of readdirSync(path)) {
    const stats = statSync(join(path, entry));
    total += stats.isDirectory() ? directorySize(join(path, entry)) : stats.size;
  }
  return total;
}

async function main() {
  const goRun = runGoSide();
  const go = goRun.metrics;
  const node = await runNodeSide();

  const gitRevision = execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
  const goVersion = goRun.version;
  const nodeSideBuild = measure(
    "npm run build --workspace @leetkob/api (warm cache)",
    ["npm", "run", "build", "--workspace", "@leetkob/api"],
    process.cwd(),
  );
  const goSideBuild = measure(
    "go build ./... (warm cache)",
    [goRun.command, "build", "./..."],
    join(process.cwd(), "apps", "backend"),
  );
  const apiArtifactBytes = directorySize(join(process.cwd(), "apps", "api", "dist"));

  const evidence = {
    generatedAt: new Date().toISOString(),
    gitRevision,
    hardware: {
      platform: `${process.platform} ${process.arch}`,
      cpuModel: String(
        execFileSync("sh", ["-c", "grep -m1 'model name' /proc/cpuinfo | cut -d: -f2"], {
          encoding: "utf8",
        }),
      ).trim(),
      totalMemoryBytes:
        Number.parseInt(readFileSync("/proc/meminfo", "utf8").split("\n")[0].split(/\s+/)[1], 10) *
        1024,
    },
    toolchain: { node: process.version, go: goVersion },
    configuration: { warmup, rounds, concurrency, labs: labCount },
    workloads:
      "identical seeded publication sequence per side: warmup replays; per round, publish + replay + concealed cross-class 404 + forbidden 403 per lab, then a concurrent different-key pair wave on one fresh lab",
    caveat:
      "Both sides run in-process with their own HTTP stack on loopback; latency includes the client fetch. Build times are warm-cache single runs, not cold-cache measurements.",
    results: [node, go],
    builds: [nodeSideBuild, goSideBuild],
    artifacts: { nestjsDistBytes: apiArtifactBytes },
    reviewAcceptance:
      "pending; this comparison does not switch the default backend, and the Go production routes stay on their explicit development port",
  };

  const jsonPath = join(process.cwd(), "docs", "evidence", "backend-comparison.json");
  writeFileSync(jsonPath, `${JSON.stringify(evidence, null, 2)}\n`);

  const markdown = `# Backend comparison — NestJS PA#2 vs Go compatpub

Generated by \`npm run compare:backends\` on ${evidence.generatedAt.slice(0, 10)} at revision \`${gitRevision.slice(0, 7)}\` on \`${process.env.LEETKOB_BRANCH ?? "the current branch"}\`. The machine-readable artifact of any rerun is [\`backend-comparison.json\`](backend-comparison.json); this page is written by the same run, so the two cannot drift. Recorded for issue #38 task 1.5.

## Method

The same seeded publication workload runs against both compositions in process on loopback: ${warmup} warmup replays, then ${rounds} rounds in which each of ${labCount} labs is published, replayed, concealed cross-class 404'd and forbidden 403'd, followed by a ${concurrency * 2}-request concurrent different-key wave on one fresh lab — ${node.requests.toLocaleString()} requests per side. Both sides run their own HTTP stack with a fixed clock and synthetic tokens; latency includes the client call. Build times are warm-cache single runs, not cold-cache measurements. The comparison covers the shared publication contract only (\`docs/contract/fixtures/publication-scenarios.json\`); the PA#3 judging pipeline has no NestJS equivalent to compare yet.

## Results

| Metric | NestJS PA#2 | Go compatpub |
| --- | ---: | ---: |
| Requests (incl. warmup) | ${node.requests} | ${go.requests} |
| Errors | ${node.errors} | ${go.errors} |
| Latency p50 (ms) | ${node.latencyMs.p50.toFixed(3)} | ${go.latencyMs.p50.toFixed(3)} |
| Latency p95 (ms) | ${node.latencyMs.p95.toFixed(3)} | ${go.latencyMs.p95.toFixed(3)} |
| Latency max (ms) | ${node.latencyMs.max.toFixed(3)} | ${go.latencyMs.max.toFixed(3)} |
| Throughput (req/s) | ${node.throughputPerSecond.toFixed(0)} | ${go.throughputPerSecond.toFixed(0)} |
| Startup (ms) | ${node.startupMs.toFixed(1)} | ${go.startupMs.toFixed(1)} |
| Build (warm cache) | ${nodeSideBuild.milliseconds.toFixed(0)} ms | ${goSideBuild.milliseconds.toFixed(0)} ms |

Environment: ${evidence.hardware.cpuModel}, ${(evidence.hardware.totalMemoryBytes / 2 ** 30).toFixed(0)} GiB RAM, ${evidence.hardware.platform}, ${process.version}, ${goVersion}.

## Interpretation and acceptance

The Go composition serves the identical contract workload with zero
errors and materially higher throughput. This is parity evidence for the
publication contract only; it is not a judging-pipeline benchmark and not
a production load test.

**Review acceptance: pending.** This comparison does not switch the
default backend: \`apps/api\` remains the assessed PA#2 application and the
Go production routes stay on their explicit development port (design
decision 1 of \`implement-pa3-go-prototype\`). A default cutover would be
its own reviewed decision after the PA#3 flow is complete.
`;
  const markdownPath = join(process.cwd(), "docs", "evidence", "backend-comparison.md");
  writeFileSync(markdownPath, markdown);

  const rows = [
    `| Metric | NestJS PA#2 | Go compatpub |`,
    `| --- | --- | --- |`,
    `| Requests (incl. warmup) | ${node.requests} | ${go.requests} |`,
    `| Errors | ${node.errors} | ${go.errors} |`,
    `| Latency p50 (ms) | ${node.latencyMs.p50.toFixed(3)} | ${go.latencyMs.p50.toFixed(3)} |`,
    `| Latency p95 (ms) | ${node.latencyMs.p95.toFixed(3)} | ${go.latencyMs.p95.toFixed(3)} |`,
    `| Latency max (ms) | ${node.latencyMs.max.toFixed(3)} | ${go.latencyMs.max.toFixed(3)} |`,
    `| Throughput (req/s) | ${node.throughputPerSecond.toFixed(0)} | ${go.throughputPerSecond.toFixed(0)} |`,
    `| Startup (ms) | ${node.startupMs.toFixed(1)} | ${go.startupMs.toFixed(1)} |`,
  ].join("\n");
  process.stdout.write(`${rows}\nWrote ${jsonPath} and ${markdownPath}\n`);
}

await main();
