import assert from "node:assert/strict";
import test from "node:test";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
} from "../../src/adapters/in-memory.js";
import { createApiApplication } from "../../src/app-factory.js";
import { issueToken } from "../../src/auth/token.js";
import type { Clock, LabRepository } from "../../src/domain/ports.js";
import type { Identity, ReadyLab } from "../../src/domain/types.js";

const secret = "test-only-http-secret-that-is-long-enough";
const nowSeconds = 1_800_000_000;
const instant = new Date("2026-10-24T01:02:03.000Z");
const lab: ReadyLab = {
  id: "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8",
  classId: "class-a",
  status: "ready",
  pendingReviews: 0,
};
const ta: Identity = { subject: "ta-1", role: "ta", classIds: ["class-a"] };

class FixedClock implements Clock {
  now(): Date {
    return new Date(instant);
  }
}

async function start(repository: LabRepository): Promise<{
  baseUrl: string;
  close: () => Promise<void>;
}> {
  const application = await createApiApplication({
    authSecret: secret,
    clock: new FixedClock(),
    idempotency: new InMemoryIdempotencyRepository(),
    labs: repository,
    nowSeconds: () => nowSeconds,
  });
  await application.listen(0, "127.0.0.1");

  return {
    baseUrl: await application.getUrl(),
    close: () => application.close(),
  };
}

test("the HTTP adapter emits documented success and problem content types", async (context) => {
  const application = await start(new InMemoryLabRepository([lab]));
  context.after(application.close);
  const path = `/labs/${lab.id}/publish`;

  const unauthorized = await fetch(`${application.baseUrl}${path}`, { method: "POST" });
  assert.equal(unauthorized.status, 401);
  assert.match(unauthorized.headers.get("content-type") ?? "", /^application\/problem\+json/);

  const token = issueToken(ta, secret, nowSeconds + 60);
  const published = await fetch(`${application.baseUrl}${path}`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "idempotency-key": "publish-request-0001",
    },
  });
  assert.equal(published.status, 200);
  assert.match(published.headers.get("content-type") ?? "", /^application\/json/);
});

test("the health endpoint returns the safe API status", async (context) => {
  const application = await start(new InMemoryLabRepository());
  context.after(application.close);

  const response = await fetch(`${application.baseUrl}/health`);
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "no-store");
  assert.deepEqual(await response.json(), { status: "ok", service: "leetkob-api" });
});
