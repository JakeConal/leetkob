import assert from "node:assert/strict";
import test from "node:test";

import {
  fetchSubmission,
  parseJudge,
  parseSubmission,
  parseSubmissionAccept,
  pollSubmissionUntilJudged,
  SubmissionPollTimeoutError,
  submitSource,
} from "../lib/student-submission.js";

const judgedBody = {
  id: "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
  labId: "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb",
  state: "judged",
  receivedAt: "2026-11-23T09:00:00.000Z",
  judge: {
    verdict: "WA",
    tests: [
      { id: "01", verdict: "AC", timeMs: 12 },
      { id: "02", verdict: "WA", timeMs: 7 },
    ],
  },
  hiddenInput: "HIDDEN-INPUT-SENTINEL",
  expectedOutput: "HIDDEN-ANSWER-SENTINEL",
  checker: { verdict: "violation", evidence: ["secret"] },
};

const queuedBody = {
  id: "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
  labId: "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb",
  state: "queued",
  receivedAt: "2026-11-23T09:00:00.000Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

test("judged submissions project only the documented safe fields", () => {
  const submission = parseSubmission(judgedBody);
  assert.deepEqual(submission, {
    id: judgedBody.id,
    labId: judgedBody.labId,
    state: "judged",
    receivedAt: judgedBody.receivedAt,
    judge: {
      verdict: "WA",
      tests: [
        { id: "01", verdict: "AC", timeMs: 12 },
        { id: "02", verdict: "WA", timeMs: 7 },
      ],
    },
  });
  const serialized = JSON.stringify(submission);
  assert.equal(serialized.includes("HIDDEN-INPUT-SENTINEL"), false);
  assert.equal(serialized.includes("HIDDEN-ANSWER-SENTINEL"), false);
  assert.equal(serialized.includes("checker"), false);
});

test("unjudged submissions have no judge member", () => {
  const submission = parseSubmission(queuedBody);
  assert.equal(submission.state, "queued");
  assert.equal("judge" in submission, false);
  assert.throws(() => parseSubmission({ ...queuedBody, judge: { verdict: "AC", tests: [] } }));
  assert.throws(() => parseSubmission({ ...judgedBody, judge: undefined }));
});

test("unknown verdicts and malformed states are refused rather than rendered", () => {
  assert.throws(() =>
    parseSubmission({
      ...judgedBody,
      judge: { verdict: "OK", tests: [] },
    }),
  );
  assert.throws(() => parseSubmission({ ...queuedBody, state: "running" }));
  assert.throws(() =>
    parseJudge({ verdict: "AC", tests: [{ id: "01", verdict: "AC", timeMs: -1 }] }),
  );
  assert.throws(() => parseSubmissionAccept({ ...queuedBody, state: "judged" }));
});

test("submission acceptance requires the fixed queued state", () => {
  assert.deepEqual(parseSubmissionAccept(queuedBody), queuedBody);
});

test("submitSource sends CSRF, idempotency and bounded source; polling reads without them", async () => {
  const observed: Array<{ url: string; init: RequestInit }> = [];
  const fetcher: typeof fetch = async (input, init) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    observed.push({ url, init: init ?? {} });
    if ((init?.method ?? "GET") === "POST") {
      return jsonResponse(queuedBody, 202);
    }
    return jsonResponse(judgedBody);
  };

  const accepted = await submitSource("bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb", "int main(){}", {
    csrfToken: "csrf-proof",
    idempotencyKey: "0123456789abcdef",
    fetcher,
  });
  assert.equal(accepted.id, queuedBody.id);

  const polled = await pollSubmissionUntilJudged({
    submissionId: queuedBody.id,
    fetcher,
    intervalMs: 1,
    maxAttempts: 2,
    wait: async () => undefined,
  });
  assert.equal(polled.state, "judged");
  assert.equal(polled.judge?.tests.length, 2);

  const post = observed[0];
  assert.ok(post);
  assert.equal(post.url, "/v2/labs/bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb/submissions");
  assert.equal(post.init.method, "POST");
  const postHeaders = new Headers(post.init.headers);
  assert.equal(postHeaders.get("x-csrf-token"), "csrf-proof");
  assert.equal(postHeaders.get("idempotency-key"), "0123456789abcdef");
  assert.deepEqual(JSON.parse(String(post.init.body)), { source: "int main(){}" });

  const read = observed[1];
  assert.ok(read);
  assert.equal(read.url, `/v2/submissions/${encodeURIComponent(queuedBody.id)}`);
  const readHeaders = new Headers(read.init.headers);
  assert.equal(readHeaders.get("x-csrf-token"), null);
  assert.equal(readHeaders.get("idempotency-key"), null);
});

test("polling stops at the first judged response and reports each state", async () => {
  const sequence = [queuedBody, { ...queuedBody, state: "judging" }, judgedBody];
  let calls = 0;
  const waits: number[] = [];
  const seen: string[] = [];
  const fetcher: typeof fetch = async () => {
    const body = sequence[Math.min(calls, sequence.length - 1)];
    calls += 1;
    return jsonResponse(body);
  };

  const submission = await pollSubmissionUntilJudged({
    submissionId: queuedBody.id,
    fetcher,
    intervalMs: 25,
    maxAttempts: 10,
    wait: async (milliseconds) => {
      waits.push(milliseconds);
    },
    onUpdate: (update) => {
      seen.push(update.state);
    },
  });

  assert.equal(submission.state, "judged");
  assert.equal(calls, 3);
  assert.deepEqual(waits, [25, 25]);
  assert.deepEqual(seen, ["queued", "judging", "judged"]);
});

test("polling is bounded and surfaces a timeout instead of hanging the screen", async () => {
  let calls = 0;
  const fetcher: typeof fetch = async () => {
    calls += 1;
    return jsonResponse(queuedBody);
  };

  await assert.rejects(
    pollSubmissionUntilJudged({
      submissionId: queuedBody.id,
      fetcher,
      maxAttempts: 2,
      wait: async () => undefined,
    }),
    SubmissionPollTimeoutError,
  );
  assert.equal(calls, 2);
});

test("fetchSubmission surfaces API problems without inventing judge data", async () => {
  const fetcher: typeof fetch = async () =>
    new Response(
      JSON.stringify({
        type: "https://leetkob.example/problems/resource-not-found",
        title: "Resource not found",
        status: 404,
        detail: "The requested resource does not exist.",
        instance: "/v2/submissions/x",
      }),
      { status: 404, headers: { "content-type": "application/problem+json" } },
    );

  await assert.rejects(
    fetchSubmission(queuedBody.id, { fetcher }),
    (error: unknown) =>
      error instanceof Error && "status" in error && (error as { status: number }).status === 404,
  );
});
