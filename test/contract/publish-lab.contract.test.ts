import assert from "node:assert/strict";
import { once } from "node:events";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import test from "node:test";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
} from "../../src/adapters/in-memory.js";
import { issueToken } from "../../src/auth/token.js";
import type { Clock, LabRepository } from "../../src/domain/ports.js";
import { PublicationService } from "../../src/domain/publication-service.js";
import type { Identity, Lab, ReadyLab } from "../../src/domain/types.js";
import { createRequestHandler } from "../../src/http/app.js";

const secret = "contract-test-secret-that-is-at-least-32-bytes";
const nowSeconds = 1_800_000_000;
const instant = new Date("2026-10-24T01:02:03.000Z");
const ownedLabId = "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8";
const pendingLabId = "f6729f3d-d07e-497d-83fb-86a85810d84d";
const foreignLabId = "39cd4be5-103f-480c-a664-79b61bafabfa";
const absentLabId = "2a6c5486-63c7-48a6-9b30-9ab3a6f67c83";
const ta: Identity = { subject: "ta-1", role: "ta", classIds: ["class-a"] };
const student: Identity = { subject: "student-1", role: "student", classIds: ["class-a"] };

class FixedClock implements Clock {
  now(): Date {
    return new Date(instant);
  }
}

function readyLab(id: string, classId = "class-a", pendingReviews = 0): ReadyLab {
  return { id, classId, status: "ready", pendingReviews };
}

function defaultLabs(): Lab[] {
  return [
    readyLab(ownedLabId),
    readyLab(pendingLabId, "class-a", 2),
    readyLab(foreignLabId, "class-b"),
  ];
}

interface RunningApplication {
  readonly baseUrl: string;
  readonly labs: InMemoryLabRepository;
  readonly close: () => Promise<void>;
  readonly tokenFor: (identity: Identity) => string;
}

async function start(
  repository: LabRepository = new InMemoryLabRepository(defaultLabs()),
): Promise<RunningApplication> {
  const labs =
    repository instanceof InMemoryLabRepository ? repository : new InMemoryLabRepository();
  const service = new PublicationService(
    repository,
    new InMemoryIdempotencyRepository(),
    new FixedClock(),
  );
  const server = createServer(
    createRequestHandler({
      publicationService: service,
      authSecret: secret,
      nowSeconds: () => nowSeconds,
    }),
  );
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address() as AddressInfo;

  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    labs,
    tokenFor: (identity) => issueToken(identity, secret, nowSeconds + 60),
    close: async () => {
      server.close();
      await once(server, "close");
    },
  };
}

interface PublishOptions {
  readonly identity?: Identity;
  readonly key?: string;
  readonly body?: string;
  readonly method?: string;
  readonly suffix?: string;
  readonly token?: string;
}

async function publish(
  application: RunningApplication,
  labId: string,
  options: PublishOptions = {},
): Promise<Response> {
  const headers = new Headers();
  const token =
    options.token ??
    (options.identity === undefined
      ? application.tokenFor(ta)
      : application.tokenFor(options.identity));
  if (token !== "") headers.set("authorization", `Bearer ${token}`);
  if (options.key !== "") headers.set("idempotency-key", options.key ?? "publish-request-0001");

  return fetch(`${application.baseUrl}/labs/${labId}/publish${options.suffix ?? ""}`, {
    method: options.method ?? "POST",
    headers,
    ...(options.body === undefined ? {} : { body: options.body }),
  });
}

async function bodyObject(response: Response): Promise<Record<string, unknown>> {
  return (await response.json()) as Record<string, unknown>;
}

async function expectProblem(
  response: Response,
  status: number,
  suffix: string,
): Promise<Record<string, unknown>> {
  assert.equal(response.status, status);
  assert.match(response.headers.get("content-type") ?? "", /^application\/problem\+json/);
  assert.equal(response.headers.get("cache-control"), "no-store");
  const body = await bodyObject(response);
  assert.equal(body["type"], `https://leetkob.example/problems/${suffix}`);
  assert.equal(body["status"], status);
  assert.equal(typeof body["title"], "string");
  assert.equal(typeof body["detail"], "string");
  assert.equal(typeof body["instance"], "string");
  return body;
}

test("successful publication returns the exact safe representation and changes state", async (context) => {
  const application = await start();
  context.after(application.close);

  const response = await publish(application, ownedLabId);
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^application\/json/);
  assert.equal(response.headers.get("cache-control"), "no-store");
  assert.deepEqual(await bodyObject(response), {
    labId: ownedLabId,
    status: "published",
    publishedAt: instant.toISOString(),
    publishedBy: ta.subject,
  });
  assert.equal((await application.labs.findById(ownedLabId))?.status, "published");
});

test("missing, malformed, tampered, and expired tokens return authentication-required", async (context) => {
  const application = await start();
  context.after(application.close);
  const valid = application.tokenFor(ta);
  const invalidTokens = [
    "",
    "not-a-token",
    `${valid.slice(0, -1)}x`,
    issueToken(ta, secret, nowSeconds),
  ];

  for (const token of invalidTokens) {
    const response = await publish(application, ownedLabId, { token });
    await expectProblem(response, 401, "authentication-required");
  }
});

test("invalid UUID, key, body, and query return the documented request problems", async (context) => {
  const application = await start();
  context.after(application.close);

  await expectProblem(await publish(application, "not-a-uuid"), 400, "invalid-request");
  await expectProblem(await publish(application, ownedLabId, { key: "" }), 400, "invalid-request");
  await expectProblem(
    await publish(application, ownedLabId, { key: "short" }),
    400,
    "invalid-request",
  );
  await expectProblem(
    await publish(application, ownedLabId, { body: "{}" }),
    400,
    "invalid-request",
  );
  await expectProblem(
    await publish(application, ownedLabId, { suffix: "?force=true" }),
    404,
    "route-not-found",
  );
  await expectProblem(
    await publish(application, ownedLabId, { method: "GET" }),
    404,
    "route-not-found",
  );
});

test("non-TA callers are forbidden and their completed rejection is replayed", async (context) => {
  const application = await start();
  context.after(application.close);
  const key = "student-request-0001";

  const first = await publish(application, ownedLabId, { identity: student, key });
  const firstBody = await expectProblem(first, 403, "ta-role-required");
  const promoted: Identity = { ...student, role: "ta" };
  const replay = await publish(application, ownedLabId, { identity: promoted, key });
  assert.deepEqual(await expectProblem(replay, 403, "ta-role-required"), firstBody);
  assert.equal((await application.labs.findById(ownedLabId))?.status, "ready");
});

test("absent and cross-class labs are indistinguishable", async (context) => {
  const application = await start();
  context.after(application.close);

  const absent = await expectProblem(
    await publish(application, absentLabId, { key: "absent-request-0001" }),
    404,
    "lab-not-found",
  );
  const foreign = await expectProblem(
    await publish(application, foreignLabId, { key: "foreign-request-0001" }),
    404,
    "lab-not-found",
  );
  assert.equal(absent["title"], foreign["title"]);
  assert.equal(absent["detail"], foreign["detail"]);
  assert.equal(JSON.stringify(foreign).includes("class-b"), false);
});

test("pending-review rejection reports a count, changes nothing, and replays after state changes", async (context) => {
  const application = await start();
  context.after(application.close);
  const key = "pending-request-0001";

  const first = await publish(application, pendingLabId, { key });
  const firstBody = await expectProblem(first, 409, "reviews-pending");
  assert.equal(firstBody["pendingReviews"], 2);
  assert.equal((await application.labs.findById(pendingLabId))?.status, "ready");

  await application.labs.save(readyLab(pendingLabId));
  const replay = await publish(application, pendingLabId, { key });
  assert.deepEqual(await expectProblem(replay, 409, "reviews-pending"), firstBody);
  assert.equal((await application.labs.findById(pendingLabId))?.status, "ready");
});

test("same-key success replays while a different key gets original publication metadata", async (context) => {
  const application = await start();
  context.after(application.close);
  const key = "success-request-0001";

  const first = await publish(application, ownedLabId, { key });
  const firstBody = await bodyObject(first);
  const replay = await publish(application, ownedLabId, { key });
  assert.deepEqual(await bodyObject(replay), firstBody);

  const different = await expectProblem(
    await publish(application, ownedLabId, { key: "success-request-0002" }),
    409,
    "lab-already-published",
  );
  assert.equal(different["labId"], ownedLabId);
  assert.equal(different["publishedAt"], instant.toISOString());
  assert.equal(different["publishedBy"], ta.subject);
});

test("concurrent different keys result in one publication", async (context) => {
  const application = await start();
  context.after(application.close);
  const responses = await Promise.all([
    publish(application, ownedLabId, { key: "concurrent-request-0001" }),
    publish(application, ownedLabId, { key: "concurrent-request-0002" }),
  ]);

  assert.deepEqual(responses.map((response) => response.status).sort(), [200, 409]);
});

test("unexpected persistence failure returns a generic problem and does not mutate the lab", async (context) => {
  const storedLabs = new InMemoryLabRepository([readyLab(ownedLabId)]);
  const repository: LabRepository = {
    findById: (id) => storedLabs.findById(id),
    save: async () => {
      throw new Error("database-password and internal table must not leak");
    },
  };
  const application = await start(repository);
  context.after(application.close);

  const response = await publish(application, ownedLabId);
  const body = await expectProblem(response, 500, "internal-error");
  assert.equal(body["detail"], "The request could not be completed.");
  assert.equal(JSON.stringify(body).includes("password"), false);
  assert.equal((await storedLabs.findById(ownedLabId))?.status, "ready");
});

test("success and error bodies contain no protected student, source, or credential data", async (context) => {
  const application = await start();
  context.after(application.close);

  const success = JSON.stringify(await bodyObject(await publish(application, ownedLabId)));
  const problem = JSON.stringify(
    await expectProblem(
      await publish(application, pendingLabId, { key: "privacy-request-0001" }),
      409,
      "reviews-pending",
    ),
  );

  for (const text of [success, problem]) {
    assert.doesNotMatch(text, /studentId|sourceCode|modelApiKey|reviewEvidence/i);
    assert.equal(text.includes(secret), false);
  }
});
