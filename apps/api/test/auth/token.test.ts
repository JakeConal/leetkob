import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import test from "node:test";

import { issueToken, verifyAuthorizationHeader } from "../../src/auth/token.js";
import type { Identity } from "../../src/domain/types.js";

const secret = "test-only-auth-secret-that-is-long-enough";
const now = 1_800_000_000;
const identity: Identity = { subject: "ta-1", role: "ta", classIds: ["class-a"] };

function signUnchecked(value: unknown): string {
  const payload = Buffer.from(JSON.stringify(value), "utf8").toString("base64url");
  const signature = createHmac("sha256", secret).update(payload, "ascii").digest("base64url");
  return `${payload}.${signature}`;
}

test("valid signed bearer claims produce an identity", () => {
  const token = issueToken(identity, secret, now + 60);
  assert.deepEqual(verifyAuthorizationHeader(`Bearer ${token}`, secret, now), identity);
});

test("missing and malformed authorization values are rejected", () => {
  assert.equal(verifyAuthorizationHeader(undefined, secret, now), undefined);
  assert.equal(verifyAuthorizationHeader("Basic abc", secret, now), undefined);
  assert.equal(verifyAuthorizationHeader("Bearer not-a-token", secret, now), undefined);
  assert.equal(verifyAuthorizationHeader("Bearer a.b.c", secret, now), undefined);
});

test("tampered tokens are rejected", () => {
  const token = issueToken(identity, secret, now + 60);
  const [payload, signature] = token.split(".");
  const tamperedPayload = Buffer.from(
    JSON.stringify({ sub: "ta-2", role: "ta", classIds: ["class-b"], exp: now + 60 }),
    "utf8",
  ).toString("base64url");

  assert.equal(
    verifyAuthorizationHeader(`Bearer ${tamperedPayload}.${signature}`, secret, now),
    undefined,
  );
  assert.notEqual(payload, tamperedPayload);
});

test("expired tokens are rejected", () => {
  const token = issueToken(identity, secret, now);
  assert.equal(verifyAuthorizationHeader(`Bearer ${token}`, secret, now), undefined);
});

test("signed but structurally invalid claims are rejected", () => {
  const invalidClaims = [
    { sub: "", role: "ta", classIds: ["class-a"], exp: now + 60 },
    { sub: "ta-1", role: "owner", classIds: ["class-a"], exp: now + 60 },
    { sub: "ta-1", role: "ta", classIds: "class-a", exp: now + 60 },
    { sub: "ta-1", role: "ta", classIds: ["class-a", "class-a"], exp: now + 60 },
    { sub: "ta-1", role: "ta", classIds: ["class-a"], exp: "later" },
  ];

  for (const claims of invalidClaims) {
    assert.equal(
      verifyAuthorizationHeader(`Bearer ${signUnchecked(claims)}`, secret, now),
      undefined,
    );
  }
});

test("short signing secrets are rejected", () => {
  assert.throws(() => issueToken(identity, "short", now + 60), /at least 32/);
  const token = signUnchecked({ sub: "ta-1", role: "ta", classIds: [], exp: now + 60 });
  assert.equal(verifyAuthorizationHeader(`Bearer ${token}`, "short", now), undefined);
});
