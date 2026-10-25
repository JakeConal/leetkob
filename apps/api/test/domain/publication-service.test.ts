import assert from "node:assert/strict";
import test from "node:test";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
} from "../../src/adapters/in-memory.js";
import type { Clock } from "../../src/domain/ports.js";
import { PublicationService, isPublished } from "../../src/domain/publication-service.js";
import type {
  Identity,
  Lab,
  ProblemResponse,
  PublishInput,
  ReadyLab,
  StoredResponse,
} from "../../src/domain/types.js";

const readyId = "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8";
const pendingId = "f6729f3d-d07e-497d-83fb-86a85810d84d";
const otherId = "39cd4be5-103f-480c-a664-79b61bafabfa";
const instant = new Date("2026-10-24T01:02:03.000Z");
const passedDeadline = "2026-10-23T22:00:00.000Z";
const futureDeadline = "2026-10-25T22:00:00.000Z";
const ta: Identity = { subject: "ta-1", role: "ta", classIds: ["class-a"] };
const student: Identity = { subject: "student-1", role: "student", classIds: ["class-a"] };

class FixedClock implements Clock {
  constructor(private readonly instant: Date) {}

  now(): Date {
    return new Date(this.instant);
  }
}

function readyLab(overrides: Partial<ReadyLab> = {}): ReadyLab {
  return {
    id: readyId,
    classId: "class-a",
    status: "ready",
    pendingReviews: 0,
    submissionDeadline: passedDeadline,
    ...overrides,
  };
}

function fixture(labs: readonly Lab[] = [readyLab()]): {
  labs: InMemoryLabRepository;
  service: PublicationService;
} {
  const repository = new InMemoryLabRepository(labs);
  return {
    labs: repository,
    service: new PublicationService(
      repository,
      new InMemoryIdempotencyRepository(),
      new FixedClock(instant),
    ),
  };
}

function input(overrides: Partial<PublishInput> = {}): PublishInput {
  return {
    identity: ta,
    labId: readyId,
    idempotencyKey: "publish-request-0001",
    instance: `/labs/${readyId}/publish`,
    ...overrides,
  };
}

function assertProblem(response: StoredResponse): asserts response is ProblemResponse {
  assert.equal(response.contentType, "application/problem+json");
}

test("only teaching assistants may publish", async () => {
  const { labs, service } = fixture();
  const response = await service.publish(input({ identity: student }));

  assert.equal(response.status, 403);
  assertProblem(response);
  assert.equal(response.body.type, "https://leetkob.example/problems/ta-role-required");
  assert.equal((await labs.findById(readyId))?.status, "ready");
});

test("absent and cross-class labs share the concealed not-found response", async () => {
  const { service } = fixture([readyLab({ id: otherId, classId: "class-b" })]);

  const crossClass = await service.publish(input({ labId: otherId }));
  const absent = await service.publish(
    input({ labId: readyId, idempotencyKey: "another-key-0002" }),
  );

  assert.equal(crossClass.status, 404);
  assert.equal(absent.status, 404);
  assertProblem(crossClass);
  assertProblem(absent);
  assert.equal(crossClass.body.type, "https://leetkob.example/problems/lab-not-found");
  assert.equal(absent.body.type, crossClass.body.type);
});

test("pending reviews block publication and the deterministic rejection is replayed", async () => {
  const pending = readyLab({ id: pendingId, pendingReviews: 2 });
  const { labs, service } = fixture([pending]);
  const request = input({ labId: pendingId });

  const first = await service.publish(request);
  await labs.save({ ...pending, pendingReviews: 0 });
  const replay = await service.publish(request);

  assert.equal(first.status, 409);
  assertProblem(first);
  assert.equal(first.body.type, "https://leetkob.example/problems/reviews-pending");
  assert.equal(first.body["pendingReviews"], 2);
  assert.deepEqual(replay, first);
  assert.equal((await labs.findById(pendingId))?.status, "ready");
});

test("the deadline gate precedes reviews and its rejection is replayed after time-state changes", async () => {
  const openLab = readyLab({ submissionDeadline: futureDeadline, pendingReviews: 2 });
  const { labs, service } = fixture([openLab]);
  const request = input();

  const first = await service.publish(request);
  assert.equal(first.status, 409);
  assertProblem(first);
  assert.equal(first.body.type, "https://leetkob.example/problems/lab-still-open");
  assert.equal(first.body["submissionDeadline"], futureDeadline);
  assert.equal(first.body["pendingReviews"], undefined);
  assert.deepEqual(await labs.findById(readyId), openLab);

  await labs.save(readyLab({ submissionDeadline: passedDeadline, pendingReviews: 0 }));
  const replay = await service.publish(request);
  assert.deepEqual(replay, first);
  assert.equal((await labs.findById(readyId))?.status, "ready");
});

test("publication is allowed exactly at the submission deadline", async () => {
  const { service } = fixture([readyLab({ submissionDeadline: instant.toISOString() })]);
  const response = await service.publish(input());

  assert.equal(response.status, 200);
});

test("an eligible lab is published atomically with safe metadata", async () => {
  const { labs, service } = fixture();
  const response = await service.publish(input());
  const stored = await labs.findById(readyId);

  assert.deepEqual(response, {
    status: 200,
    contentType: "application/json",
    body: {
      labId: readyId,
      status: "published",
      publishedAt: instant.toISOString(),
      publishedBy: ta.subject,
    },
  });
  assert.ok(isPublished(stored));
  assert.equal(stored.publicationKey, "publish-request-0001");
});

test("same-key success retries return the original representation", async () => {
  const { service } = fixture();
  const first = await service.publish(input());
  const replay = await service.publish(input());

  assert.deepEqual(replay, first);
});

test("a different key after publication returns original publication metadata", async () => {
  const { service } = fixture();
  await service.publish(input());
  const response = await service.publish(input({ idempotencyKey: "publish-request-0002" }));

  assert.equal(response.status, 409);
  assertProblem(response);
  assert.equal(response.body.type, "https://leetkob.example/problems/lab-already-published");
  assert.equal(response.body["publishedAt"], instant.toISOString());
  assert.equal(response.body["publishedBy"], ta.subject);
});

test("concurrent different-key requests perform only one transition", async () => {
  const { service } = fixture();
  const responses = await Promise.all([
    service.publish(input({ idempotencyKey: "publish-concurrent-1" })),
    service.publish(input({ idempotencyKey: "publish-concurrent-2" })),
  ]);

  assert.deepEqual(responses.map((response) => response.status).sort(), [200, 409]);
});

test("a failed save leaves the stored lab unchanged", async () => {
  const original = readyLab();
  const labs = new InMemoryLabRepository([original]);
  const failingRepository = {
    findById: (id: string): Promise<Lab | undefined> => labs.findById(id),
    save: async (): Promise<void> => {
      throw new Error("simulated persistence failure");
    },
  };
  const service = new PublicationService(
    failingRepository,
    new InMemoryIdempotencyRepository(),
    new FixedClock(instant),
  );

  await assert.rejects(service.publish(input()), /simulated persistence failure/);
  assert.deepEqual(await labs.findById(readyId), original);
});
