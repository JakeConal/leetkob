import assert from "node:assert/strict";
import test from "node:test";

import { fetchHealth } from "../lib/health.js";
import { isPublicationBlocked } from "../lib/review-data.js";

test("health query accepts the documented safe response", async () => {
  const fetcher: typeof fetch = async () =>
    new Response(JSON.stringify({ status: "ok", service: "leetkob-api" }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });

  assert.deepEqual(await fetchHealth(fetcher), { status: "ok", service: "leetkob-api" });
});

test("health query rejects failed and malformed responses", async () => {
  const unavailable: typeof fetch = async () => new Response(null, { status: 503 });
  const malformed: typeof fetch = async () =>
    new Response(JSON.stringify({ status: "ok", service: "unexpected" }), { status: 200 });

  await assert.rejects(fetchHealth(unavailable), /status 503/);
  await assert.rejects(fetchHealth(malformed), /did not match/);
});

test("publication stays blocked until both FAQ-12 preconditions pass", () => {
  assert.equal(isPublicationBlocked({ deadlinePassed: false, pendingReviews: 0 }), true);
  assert.equal(isPublicationBlocked({ deadlinePassed: true, pendingReviews: 1 }), true);
  assert.equal(isPublicationBlocked({ deadlinePassed: false, pendingReviews: 1 }), true);
  assert.equal(isPublicationBlocked({ deadlinePassed: true, pendingReviews: 0 }), false);
});
