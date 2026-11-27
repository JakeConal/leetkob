import assert from "node:assert/strict";
import test from "node:test";

import { filterLabs, labAvailability } from "../lib/lab-catalog.js";
import { listLabs, type LabSummary } from "../lib/live/api.js";

const lab: LabSummary = {
  id: "11111111-1111-4111-8111-111111111111",
  classId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  title: "Array sum",
  opensAt: "2026-11-27T08:00:00.000Z",
  deadline: "2026-11-27T09:00:00.000Z",
  revision: 1,
};

test("catalog availability includes the opening time and excludes the deadline", () => {
  const opens = Date.parse(lab.opensAt);
  const deadline = Date.parse(lab.deadline);
  assert.equal(labAvailability(lab, opens - 1), "Upcoming");
  assert.equal(labAvailability(lab, opens), "Open");
  assert.equal(labAvailability(lab, deadline - 1), "Open");
  assert.equal(labAvailability(lab, deadline), "Closed");
});

test("search and availability intersect without changing the lab collection", () => {
  const closed = { ...lab, id: "closed", title: "Binary search", deadline: lab.opensAt };
  const labs = [lab, closed];
  const now = Date.parse(lab.opensAt);
  assert.deepEqual(filterLabs(labs, " ARRAY ", "Open", now), [lab]);
  assert.deepEqual(filterLabs(labs, "Binary", "Open", now), []);
  assert.deepEqual(filterLabs(labs, "closed", "All labs", now), [closed]);
  assert.deepEqual(labs, [lab, closed]);
});

test("catalog pagination preserves the opaque cursor and safe lab projection", async (t) => {
  let requested = "";
  t.mock.method(globalThis, "fetch", async (input: string, init?: RequestInit) => {
    requested = input;
    assert.equal(init?.cache, "no-store");
    assert.equal(new Headers(init?.headers).has("X-CSRF-Token"), false);
    return Response.json({
      items: [{ ...lab, source: "private", findings: ["private"] }],
      nextCursor: "second-page",
    });
  });
  const page = await listLabs(lab.classId, "next page&cursor");
  const url = new URL(requested, "http://localhost");
  assert.equal(url.pathname, `/v2/classes/${lab.classId}/labs`);
  assert.equal(url.searchParams.get("cursor"), "next page&cursor");
  assert.deepEqual(page, { items: [lab], nextCursor: "second-page" });
});
