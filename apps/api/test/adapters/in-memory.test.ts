import assert from "node:assert/strict";
import test from "node:test";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
} from "../../src/adapters/in-memory.js";
import type { ReadyLab, StoredResponse } from "../../src/domain/types.js";

const lab: ReadyLab = {
  id: "0d0f366c-f9cf-45e8-bbf2-ab1b8e370ec8",
  classId: "class-a",
  status: "ready",
  pendingReviews: 0,
  submissionDeadline: "2026-10-23T22:00:00.000Z",
};

test("lab repository isolates saved and returned values", async () => {
  const repository = new InMemoryLabRepository([lab]);
  const first = await repository.findById(lab.id);

  assert.deepEqual(first, lab);
  assert.notStrictEqual(first, lab);

  const replacement: ReadyLab = { ...lab, pendingReviews: 2 };
  await repository.save(replacement);
  assert.deepEqual(await repository.findById(lab.id), replacement);
});

test("idempotency repository preserves the first exact response", async () => {
  const repository = new InMemoryIdempotencyRepository();
  const first: StoredResponse = {
    status: 409,
    contentType: "application/problem+json",
    body: {
      type: "https://leetkob.example/problems/reviews-pending",
      title: "Reviews pending",
      status: 409,
      detail: "Two reviews remain.",
      instance: "/labs/example/publish",
      pendingReviews: 2,
    },
  };
  const later: StoredResponse = {
    status: 200,
    contentType: "application/json",
    body: {
      labId: lab.id,
      status: "published",
      publishedAt: "2026-10-24T00:00:00.000Z",
      publishedBy: "ta-1",
    },
  };

  await repository.save("scope", first);
  await repository.save("scope", later);

  const replay = await repository.find("scope");
  assert.deepEqual(replay, first);
  assert.notStrictEqual(replay, first);
});
