import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { ApiConnectionError } from "../lib/live/api.js";
import { liveApiPathTemplates } from "../lib/live/paths.js";
import { CSRF_STORAGE_KEY, clearProtectedSession } from "../lib/live/session.js";
import { projectStudentLab, projectStudentStatus } from "../lib/live/view-rules.js";
import { PublishControl } from "../components/live/publish-control.js";
import { StudentSubmissionStatus } from "../components/live/student-submission-status.js";
import { LiveError } from "../components/live/live-status.js";

const labId = "1b0a6a26-0f9e-4d3d-a2e1-5c7f7e9e1f01";
const submissionId = "57000000-0000-4000-8000-000000000001";

function keysOf(value: unknown): string[] {
  if (Array.isArray(value)) return value.flatMap((item) => keysOf(item));
  if (typeof value !== "object" || value === null) return [];
  return Object.keys(value).flatMap((key) => [
    key,
    ...keysOf((value as Record<string, unknown>)[key]),
  ]);
}

test("student status payload does not keep finding fields", () => {
  const secret = "std::sort(a.begin(), a.end());";
  const view = projectStudentStatus({
    id: submissionId,
    labId,
    state: "judged",
    receivedAt: "2026-11-24T01:00:00.000Z",
    judge: {
      verdict: "AC",
      tests: [{ id: "sample-1", verdict: "AC", timeMs: 12, evidence: [{ line: 3, text: secret }] }],
      checkerVerdict: "violation",
    },
    checkerVerdict: "violation",
    evidence: [{ line: 3, text: secret }],
    deduction: 3,
    source: secret,
    unsureCause: "timeout",
    decision: { verdict: "violation", reason: "banned call" },
    findings: [{ checkerVerdict: "unsure" }],
  });
  const forbidden = [
    "checkerVerdict",
    "evidence",
    "deduction",
    "source",
    "unsureCause",
    "decision",
    "findings",
  ];
  assert.deepEqual(
    keysOf(view).filter((key) => forbidden.includes(key)),
    [],
  );
  assert.equal(JSON.stringify(view).includes(secret), false);
  assert.equal(JSON.stringify(view).includes("violation"), false);
  assert.equal(JSON.stringify(view).includes("unsure"), false);
  assert.deepEqual(view, {
    id: submissionId,
    labId,
    state: "judged",
    receivedAt: "2026-11-24T01:00:00.000Z",
    judge: { verdict: "AC", tests: [{ id: "sample-1", verdict: "AC", timeMs: 12 }] },
  });

  const html = renderToStaticMarkup(
    createElement(StudentSubmissionStatus, {
      payload: {
        id: submissionId,
        labId,
        state: "judged",
        receivedAt: "2026-11-24T01:00:00.000Z",
        source: secret,
        deduction: 4,
        judge: { verdict: "AC", tests: [{ id: "sample-1", verdict: "WA", timeMs: 12 }] },
      },
    }),
  );
  assert.match(html, /judged/);
  assert.match(html, />12</);
  assert.equal(html.includes(secret), false);
  assert.equal(html.includes("deduction"), false);
});

test("a model verdict is not rendered on the student status view", () => {
  const html = renderToStaticMarkup(
    createElement(StudentSubmissionStatus, {
      payload: {
        id: submissionId,
        labId,
        state: "judged",
        receivedAt: "2026-11-24T01:00:00.000Z",
        source: "SECRET_SOURCE",
        judge: { verdict: "violation", tests: [] },
      },
    }),
  );
  assert.match(html, /This result cannot be shown/);
  assert.equal(html.includes("SECRET_SOURCE"), false);
  assert.equal(html.includes("violation"), false);
});

test("student lab view keeps sample tests and drops hidden tests", () => {
  const lab = projectStudentLab({
    id: labId,
    classId: "2c1b7b37-1a0f-4e4e-b3f2-6d8f8f0f2f12",
    title: "Sort and merge",
    statement: "Sort the numbers.",
    opensAt: "2026-11-20T04:00:00.000Z",
    deadline: "2026-11-25T15:00:00.000Z",
    revision: 1,
    modelNotice: "A person approves every deduction.",
    constraints: [{ id: "C1", text: "Do not call `std::sort`.", deduction: 3 }],
    limits: { cpuSeconds: 1, memoryMiB: 256 },
    sampleTests: [
      { id: "sample", input: "1\n", expectedOutput: "1\n", sample: true },
      { id: "hidden", input: "secret-input", expectedOutput: "secret-output", sample: false },
    ],
    hiddenTests: [{ id: "other", input: "secret-input", expectedOutput: "secret-output" }],
    findings: [{ evidence: [{ text: "secret-input" }] }],
  });
  assert.deepEqual(lab.sampleTests, [{ id: "sample", input: "1\n", expectedOutput: "1\n" }]);
  assert.equal(lab.modelNotice, "A person approves every deduction.");
  assert.equal(JSON.stringify(lab).includes("secret-input"), false);
  assert.equal(JSON.stringify(lab).includes("secret-output"), false);
  assert.equal("findings" in lab, false);
  assert.equal("hiddenTests" in lab, false);
});

test("publish control stays disabled while pending reviews remain", () => {
  const html = renderToStaticMarkup(
    createElement(PublishControl, {
      reviewPending: 3,
      processingPending: false,
      deadlinePassed: true,
    }),
  );
  assert.match(html, /<button[^>]*disabled/);
  assert.match(html, /Publish lab/);
  assert.match(html, /Publication blocked/);
  assert.match(html, /Every review must be resolved/);
  assert.match(html, /\b3\b/);
});

test("publish control stays disabled when processing is unfinished and when nothing is pending", () => {
  const processing = renderToStaticMarkup(
    createElement(PublishControl, {
      reviewPending: 0,
      processingPending: true,
      deadlinePassed: true,
    }),
  );
  assert.match(processing, /<button[^>]*disabled/);
  assert.match(processing, /Processing incomplete/);

  const clear = renderToStaticMarkup(
    createElement(PublishControl, {
      reviewPending: 0,
      processingPending: false,
      deadlinePassed: true,
    }),
  );
  assert.match(clear, /<button[^>]*disabled/);
  assert.match(clear, /Production publication is unavailable/);
});

test("a failed API call renders a connection error and no judged submission", () => {
  const html = renderToStaticMarkup(createElement(LiveError, { error: new ApiConnectionError() }));
  assert.match(html, /Could not reach the API/);
  assert.doesNotMatch(html, /queued|judged|Accepted/);
});

test("logout clears the session token and the protected cache", () => {
  const store = new Map<string, string>([
    [CSRF_STORAGE_KEY, "csrf-token"],
    ["unrelated", "keep"],
  ]);
  const records = ["submitted source", "review evidence"];
  clearProtectedSession(
    {
      removeItem(key: string) {
        store.delete(key);
      },
    },
    {
      clear() {
        records.length = 0;
      },
    },
  );
  assert.equal(store.has(CSRF_STORAGE_KEY), false);
  assert.equal(store.get("unrelated"), "keep");
  assert.deepEqual(records, []);
});

test("live client paths are published OpenAPI paths", () => {
  const spec = JSON.parse(
    readFileSync(new URL("../../../docs/contract/pa3-openapi.json", import.meta.url), "utf8"),
  ) as { paths: Record<string, unknown> };
  for (const template of Object.values(liveApiPathTemplates)) {
    assert.equal(Object.hasOwn(spec.paths, template), true, template);
  }
});
