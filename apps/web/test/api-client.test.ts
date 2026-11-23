import assert from "node:assert/strict";
import test from "node:test";

import {
  apiRequest,
  ApiError,
  createIdempotencyKey,
  parseProblemBody,
  summarizeError,
} from "../lib/api-client.js";
import {
  cacheProtected,
  endSession,
  getLastEndReason,
  getSession,
  readProtected,
  resetProtectedStateForTests,
  startSession,
} from "../lib/protected-state.js";
import { fetchCurrentUser, signIn } from "../lib/session-client.js";

function jsonResponse(
  body: unknown,
  status = 200,
  headers: Readonly<Record<string, string>> = {},
): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...headers },
  });
}

const problemBody = {
  type: "https://leetkob.example/problems/invalid-request",
  title: "Invalid request",
  status: 400,
  detail: "The request is not valid for this operation.",
  instance: "/v2/classes/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/labs",
  conflictKind: "idempotency-content-mismatch",
};

const sessionUser = {
  id: "11111111-1111-4111-8111-111111111111",
  displayName: "PA3 TA One",
  role: "ta",
} as const;

function seedSession(): void {
  resetProtectedStateForTests();
  startSession({
    user: sessionUser,
    csrfToken: "csrf-proof",
    expiresAt: "2026-11-23T20:00:00.000Z",
  });
}

test("problem details are parsed with documented extensions", async () => {
  const error = new ApiError(400, parseProblemBody(problemBody));
  assert.equal(error.status, 400);
  assert.equal(error.problem?.title, "Invalid request");
  assert.equal(error.problem?.detail, "The request is not valid for this operation.");
  assert.equal(error.problem?.extensions["conflictKind"], "idempotency-content-mismatch");
  assert.equal(error.isUnauthenticated, false);
});

test("malformed problem bodies never masquerade as contract problems", () => {
  assert.equal(parseProblemBody({ type: "x", title: "y", status: "400" }), null);
  assert.equal(parseProblemBody({ ...problemBody, instance: 42 }), null);
  assert.equal(parseProblemBody("not-json"), null);
});

test("non-JSON failures surface the HTTP status without inventing a problem body", async () => {
  const fetcher: typeof fetch = async () =>
    new Response("<html>bad gateway</html>", {
      status: 502,
      headers: { "content-type": "text/html" },
    });

  await assert.rejects(
    apiRequest("/v2/classes", { fetcher }),
    (error: unknown) =>
      error instanceof ApiError &&
      error.status === 502 &&
      error.problem === null &&
      error.message === "Request failed with status 502",
  );
});

test("mutations inject X-CSRF-Token and Idempotency-Key; reads do not", async () => {
  const observed: RequestInit[] = [];
  const fetcher: typeof fetch = async (_input, init) => {
    observed.push(init ?? {});
    return jsonResponse({}, 201);
  };

  await apiRequest("/v2/classes/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/labs", {
    method: "POST",
    body: { title: "x" },
    csrfToken: "csrf-proof",
    idempotencyKey: "0123456789abcdef",
    fetcher,
  });
  await apiRequest("/v2/auth/me", { method: "GET", csrfToken: "must-not-leak", fetcher });

  const mutationHeaders = new Headers(observed[0]?.headers);
  assert.equal(mutationHeaders.get("x-csrf-token"), "csrf-proof");
  assert.equal(mutationHeaders.get("idempotency-key"), "0123456789abcdef");
  assert.equal(mutationHeaders.get("content-type"), "application/json");

  const readHeaders = new Headers(observed[1]?.headers);
  assert.equal(readHeaders.get("x-csrf-token"), null);
  assert.equal(readHeaders.get("content-type"), null);
});

test("401 handling clears the in-memory session and protected cache", async () => {
  seedSession();
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);
  assert.notEqual(readProtected("classes"), null);

  const fetcher: typeof fetch = async () =>
    new Response(
      JSON.stringify({
        type: "https://leetkob.example/problems/authentication-required",
        title: "Authentication required",
        status: 401,
        detail: "Authentication is required to access this resource.",
        instance: "/v2/auth/me",
      }),
      { status: 401, headers: { "content-type": "application/problem+json" } },
    );

  await assert.rejects(
    fetchCurrentUser(fetcher),
    (error: unknown) =>
      error instanceof ApiError && error.status === 401 && error.isUnauthenticated,
  );

  assert.equal(getSession(), null);
  assert.equal(readProtected("classes"), null);
  assert.equal(getLastEndReason(), "authentication-required");
});

test("sign-in stores the session in memory only", async () => {
  resetProtectedStateForTests();
  const fetcher: typeof fetch = async () =>
    jsonResponse({
      user: sessionUser,
      csrfToken: "fresh-proof",
      expiresAt: "2026-11-23T20:00:00.000Z",
    });

  const session = await signIn("ta1@leetkob.local", "secret", fetcher);
  assert.equal(session.csrfToken, "fresh-proof");
  assert.equal(getSession()?.user.id, sessionUser.id);
});

test("idempotency keys are 32 visible ASCII characters", () => {
  const key = createIdempotencyKey(new Uint8Array(Array.from({ length: 16 }, (_, i) => i)));
  assert.equal(key.length, 32);
  assert.match(key, /^[!-~]{8,128}$/);
  assert.notEqual(createIdempotencyKey(), createIdempotencyKey());
});

test("error summaries expose the documented problem members", () => {
  const summary = summarizeError(new ApiError(400, parseProblemBody(problemBody), 60));
  assert.deepEqual(summary, {
    title: "Invalid request",
    detail: "The request is not valid for this operation.",
    status: 400,
    type: "https://leetkob.example/problems/invalid-request",
    instance: "/v2/classes/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/labs",
    retryAfterSeconds: 60,
  });
  endSession("logout");
});
