import assert from "node:assert/strict";
import test from "node:test";

import { isAllowedV2Request, isUuid } from "../lib/proxy.js";

const CLASS_ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const LAB_ID = "2c1b7b37-1a0f-4e4e-b3f2-6d8f8f0f2f12";

test("only the documented PA#3 browser routes are proxied", () => {
  assert.equal(isAllowedV2Request("GET", ["auth", "me"]), true);
  assert.equal(isAllowedV2Request("POST", ["auth", "login"]), true);
  assert.equal(isAllowedV2Request("POST", ["auth", "logout"]), true);
  assert.equal(isAllowedV2Request("GET", ["classes"]), true);
  assert.equal(isAllowedV2Request("GET", ["classes", CLASS_ID, "labs"]), true);
  assert.equal(isAllowedV2Request("POST", ["classes", CLASS_ID, "labs"]), true);
  assert.equal(isAllowedV2Request("GET", ["labs", LAB_ID]), true);
  assert.equal(isAllowedV2Request("GET", ["labs", LAB_ID, "submissions"]), true);
  assert.equal(isAllowedV2Request("POST", ["labs", LAB_ID, "submissions"]), true);
  assert.equal(isAllowedV2Request("GET", ["submissions", LAB_ID]), true);

  assert.equal(isAllowedV2Request("GET", ["auth", "login"]), false);
  assert.equal(isAllowedV2Request("POST", ["auth", "me"]), false);
  assert.equal(isAllowedV2Request("POST", ["classes"]), false);
  assert.equal(isAllowedV2Request("DELETE", ["labs", LAB_ID]), false);
  assert.equal(isAllowedV2Request("GET", ["classes", "not-a-uuid", "labs"]), false);
  assert.equal(isAllowedV2Request("GET", ["labs", "not-a-uuid"]), false);
  assert.equal(isAllowedV2Request("GET", ["classes", CLASS_ID, "labs", "extra"]), false);
  assert.equal(isAllowedV2Request("DELETE", ["labs", LAB_ID, "submissions"]), false);
  assert.equal(isAllowedV2Request("POST", ["submissions", LAB_ID]), false);
  assert.equal(isAllowedV2Request("GET", ["submissions", "not-a-uuid"]), false);
  assert.equal(isAllowedV2Request("GET", ["labs", LAB_ID, "submissions", "extra"]), false);
  assert.equal(isAllowedV2Request("GET", []), false);
  assert.equal(isAllowedV2Request("GET", ["auth", ""]), false);
});

test("UUIDs accept versions 1-5 and reject other shapes", () => {
  assert.equal(isUuid(CLASS_ID), true);
  assert.equal(isUuid(LAB_ID), true);
  assert.equal(isUuid(LAB_ID.toUpperCase()), true);
  assert.equal(isUuid("2c1b7b37-1a0f-4e4e-b3f2-6d8f8f0f2f1"), false);
  assert.equal(isUuid("2c1b7b37-1a0f-6e4e-b3f2-6d8f8f0f2f12"), false);
  assert.equal(isUuid("../../etc/passwd"), false);
});
