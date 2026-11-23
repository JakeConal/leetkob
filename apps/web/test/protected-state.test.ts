import assert from "node:assert/strict";
import test from "node:test";

import type { ProtectedSession } from "../lib/protected-state.js";
import {
  adoptCurrentUser,
  cacheProtected,
  endSession,
  getCsrfToken,
  getLastEndReason,
  getSession,
  readProtected,
  resetProtectedStateForTests,
  startSession,
  subscribeProtectedState,
} from "../lib/protected-state.js";

const userA = {
  id: "11111111-1111-4111-8111-111111111111",
  displayName: "PA3 TA One",
  role: "ta",
} as const;

const userB = {
  id: "22222222-2222-4222-8222-222222222222",
  displayName: "PA3 TA Two",
  role: "ta",
} as const;

function sessionFor(user: typeof userA | typeof userB, token: string): ProtectedSession {
  return { user, csrfToken: token, expiresAt: "2026-11-23T20:00:00.000Z" };
}

test("logout clears the session, CSRF proof and protected cache", () => {
  resetProtectedStateForTests();
  startSession(sessionFor(userA, "csrf-a"));
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);
  assert.notEqual(readProtected("classes"), null);
  assert.equal(getCsrfToken(), "csrf-a");

  endSession("logout");

  assert.equal(getSession(), null);
  assert.equal(getCsrfToken(), null);
  assert.equal(readProtected("classes"), null);
  assert.equal(getLastEndReason(), "logout");
});

test("a different account clears protected state and reports the change", () => {
  resetProtectedStateForTests();
  startSession(sessionFor(userA, "csrf-a"));
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);

  const result = startSession(sessionFor(userB, "csrf-b"));

  assert.equal(result.accountChanged, true);
  assert.equal(readProtected("classes"), null);
  assert.equal(getSession()?.user.id, userB.id);
  assert.equal(getCsrfToken(), "csrf-b");
});

test("signing in again as the same account keeps its protected cache", () => {
  resetProtectedStateForTests();
  startSession(sessionFor(userA, "csrf-a"));
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);

  const result = startSession(sessionFor(userA, "csrf-a-rotated"));

  assert.equal(result.accountChanged, false);
  assert.notEqual(readProtected("classes"), null);
});

test("/v2/auth/me disagreeing with the in-memory account wipes protected state", () => {
  resetProtectedStateForTests();
  startSession(sessionFor(userA, "csrf-a"));
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);

  const outcome = adoptCurrentUser(userB);

  assert.equal(outcome, "account-changed");
  assert.equal(getSession(), null);
  assert.equal(readProtected("classes"), null);
  assert.equal(getLastEndReason(), "account-change");
});

test("the same account reported by /v2/auth/me keeps protected state", () => {
  resetProtectedStateForTests();
  startSession(sessionFor(userA, "csrf-a"));
  cacheProtected("classes", [{ id: "class-a", name: "PA3 Class A" }]);

  assert.equal(adoptCurrentUser(userA), "kept");
  assert.notEqual(readProtected("classes"), null);
});

test("subscribers are notified on session changes", () => {
  resetProtectedStateForTests();
  let calls = 0;
  const unsubscribe = subscribeProtectedState(() => {
    calls += 1;
  });

  startSession(sessionFor(userA, "csrf-a"));
  endSession("authentication-required");
  unsubscribe();
  endSession("logout");

  assert.equal(calls, 2);
});

test("protected values cannot be cached without a session", () => {
  resetProtectedStateForTests();
  assert.throws(() => cacheProtected("classes", []), /without an authenticated session/);
});
