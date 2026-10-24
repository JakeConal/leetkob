import assert from "node:assert/strict";
import test from "node:test";

test("the Node test harness is available", () => {
  assert.equal(1 + 1, 2);
});
