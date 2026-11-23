import assert from "node:assert/strict";
import test from "node:test";

import {
  LAB_BOUNDS,
  parseConstraintLines,
  parseTestItems,
  validateLabDraft,
  type LabDraftInput,
} from "../lib/lab-form.js";

function validDraft(overrides: Partial<LabDraftInput> = {}): LabDraftInput {
  return {
    title: "Sorting lab",
    statement: "Implement merge sort.",
    opensAt: "2026-11-20T04:00",
    deadline: "2026-11-25T15:00",
    constraintsText: "C1 | Do not call `std::sort`. | 3\nC2 | Implement merge sort. | 4",
    testsText: JSON.stringify([
      { id: "01", input: "3\n3 1 2\n", expectedOutput: "1 2 3\n", sample: true },
    ]),
    cpuSeconds: "1",
    memoryMiB: "256",
    ...overrides,
  };
}

test("a valid draft maps to the documented LabCreateRequest shape", () => {
  const result = validateLabDraft(validDraft());
  assert.equal(result.ok, true);
  if (!result.ok) return;
  assert.equal(result.value.opensAt, new Date("2026-11-20T04:00").toISOString());
  assert.equal(result.value.deadline, new Date("2026-11-25T15:00").toISOString());
  assert.deepEqual(result.value.constraints, [
    { id: "C1", text: "Do not call `std::sort`.", deduction: 3 },
    { id: "C2", text: "Implement merge sort.", deduction: 4 },
  ]);
  assert.deepEqual(result.value.limits, { cpuSeconds: 1, memoryMiB: 256 });
  assert.equal(result.value.tests.length, 1);
});

test("title and statement bounds are enforced", () => {
  const emptyTitle = validateLabDraft(validDraft({ title: "   " }));
  assert.equal(emptyTitle.ok, false);
  if (!emptyTitle.ok) assert.match(emptyTitle.errors["title"] ?? "", /1-200/);

  const longTitle = validateLabDraft(
    validDraft({ title: "x".repeat(LAB_BOUNDS.title.maxLength + 1) }),
  );
  assert.equal(longTitle.ok, false);
  if (!longTitle.ok) assert.match(longTitle.errors["title"] ?? "", /1-200/);

  const longStatement = validateLabDraft(
    validDraft({ statement: "s".repeat(LAB_BOUNDS.statement.maxLength + 1) }),
  );
  assert.equal(longStatement.ok, false);
  if (!longStatement.ok) assert.match(longStatement.errors["statement"] ?? "", /65536/);
});

test("the deadline must be strictly after the opening time", () => {
  const same = validateLabDraft(
    validDraft({ opensAt: "2026-11-20T04:00", deadline: "2026-11-20T04:00" }),
  );
  assert.equal(same.ok, false);
  if (!same.ok) assert.match(same.errors["deadline"] ?? "", /strictly after/);

  const before = validateLabDraft(
    validDraft({ opensAt: "2026-11-21T04:00", deadline: "2026-11-20T04:00" }),
  );
  assert.equal(before.ok, false);
});

test("constraint lines reject bad IDs, duplicates, counts and deductions", () => {
  const badId = parseConstraintLines("1C | text | 3");
  assert.equal(badId.ok, false);
  if (!badId.ok) assert.match(badId.error, /ID must start with a letter/);

  const duplicate = parseConstraintLines("C1 | one | 3\nC1 | two | 4");
  assert.equal(duplicate.ok, false);
  if (!duplicate.ok) assert.match(duplicate.error, /duplicated/);

  const tooMany = Array.from({ length: 21 }, (_, index) => `C${index} | text | 1`).join("\n");
  const count = parseConstraintLines(tooMany);
  assert.equal(count.ok, false);
  if (!count.ok) assert.match(count.error, /At most 20/);

  const deductionTooHigh = parseConstraintLines("C1 | text | 10.01");
  assert.equal(deductionTooHigh.ok, false);
  if (!deductionTooHigh.ok) assert.match(deductionTooHigh.error, /between 0 and 10/);

  const deductionPrecision = parseConstraintLines("C1 | text | 3.456");
  assert.equal(deductionPrecision.ok, false);
  if (!deductionPrecision.ok) assert.match(deductionPrecision.error, /two decimal places/);

  const missingSeparator = parseConstraintLines("C1 | text");
  assert.equal(missingSeparator.ok, false);

  const empty = parseConstraintLines("   ");
  assert.equal(empty.ok, false);
});

test("constraint text may contain pipes and keeps the first/last separators", () => {
  const parsed = parseConstraintLines("C1 | avoid a|b alternation | 2.5");
  assert.equal(parsed.ok, true);
  if (!parsed.ok) return;
  assert.deepEqual(parsed.constraints, [
    { id: "C1", text: "avoid a|b alternation", deduction: 2.5 },
  ]);
});

test("tests JSON honours count, shape, unique IDs and payload bounds", () => {
  const notJson = parseTestItems("not json");
  assert.equal(notJson.ok, false);

  const notArray = parseTestItems('{"id":"01"}');
  assert.equal(notArray.ok, false);

  const empty = parseTestItems("[]");
  assert.equal(empty.ok, false);

  const missingField = parseTestItems('[{"id":"01","input":"","sample":true}]');
  assert.equal(missingField.ok, false);

  const unknownField = parseTestItems(
    '[{"id":"01","input":"","expectedOutput":"","sample":true,"secret":true}]',
  );
  assert.equal(unknownField.ok, false);

  const duplicate = parseTestItems(
    JSON.stringify([
      { id: "01", input: "", expectedOutput: "", sample: true },
      { id: "01", input: "", expectedOutput: "", sample: false },
    ]),
  );
  assert.equal(duplicate.ok, false);

  const tooMany = parseTestItems(
    JSON.stringify(
      Array.from({ length: LAB_BOUNDS.tests.maxItems + 1 }, (_, index) => ({
        id: String(index),
        input: "",
        expectedOutput: "",
        sample: false,
      })),
    ),
  );
  assert.equal(tooMany.ok, false);

  const oversized = parseTestItems(
    JSON.stringify([
      {
        id: "01",
        input: "x".repeat(LAB_BOUNDS.tests.payloadMaxBytes + 1),
        expectedOutput: "",
        sample: false,
      },
    ]),
  );
  assert.equal(oversized.ok, false);
});

test("limits must be whole numbers inside the documented ranges", () => {
  const cpuBelow = validateLabDraft(validDraft({ cpuSeconds: "0" }));
  assert.equal(cpuBelow.ok, false);
  if (!cpuBelow.ok) assert.match(cpuBelow.errors["cpuSeconds"] ?? "", /1 to 5/);

  const cpuAbove = validateLabDraft(validDraft({ cpuSeconds: "6" }));
  assert.equal(cpuAbove.ok, false);

  const cpuFraction = validateLabDraft(validDraft({ cpuSeconds: "1.5" }));
  assert.equal(cpuFraction.ok, false);

  const memoryBelow = validateLabDraft(validDraft({ memoryMiB: "63" }));
  assert.equal(memoryBelow.ok, false);
  if (!memoryBelow.ok) assert.match(memoryBelow.errors["memoryMiB"] ?? "", /64 to 512/);

  const memoryAbove = validateLabDraft(validDraft({ memoryMiB: "513" }));
  assert.equal(memoryAbove.ok, false);
});

test("multiple invalid fields are reported together", () => {
  const result = validateLabDraft(
    validDraft({ title: "", constraintsText: "bad", testsText: "[]", cpuSeconds: "" }),
  );
  assert.equal(result.ok, false);
  if (result.ok) return;
  assert.deepEqual(Object.keys(result.errors).sort(), [
    "constraints",
    "cpuSeconds",
    "tests",
    "title",
  ]);
});
