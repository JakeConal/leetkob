/**
 * Pure client-side mirror of the documented PA#3 lab-creation bounds
 * (`docs/contract/pa3-api.md`, "Lab creation" and the `LabCreateRequest`
 * schema). The server remains authoritative; these checks only prevent
 * avoidable round trips and focus the first invalid field.
 */

import type { Constraint, LabCreateRequest, Limits, TestItem } from "./api-types";

export const LAB_BOUNDS = {
  title: { maxLength: 200 },
  statement: { maxLength: 65536 },
  constraints: {
    minItems: 1,
    maxItems: 20,
    idPattern: /^[A-Za-z][A-Za-z0-9_-]{0,31}$/,
    textMaxLength: 4096,
    deductionMin: 0,
    deductionMax: 10,
  },
  tests: {
    minItems: 1,
    maxItems: 500,
    idMaxLength: 64,
    payloadMaxBytes: 10 * 1024 * 1024,
  },
  limits: {
    cpuSeconds: { min: 1, max: 5 },
    memoryMiB: { min: 64, max: 512 },
  },
} as const;

export interface LabDraftInput {
  readonly title: string;
  readonly statement: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly constraintsText: string;
  readonly testsText: string;
  readonly cpuSeconds: string;
  readonly memoryMiB: string;
}

export type LabDraftErrors = Readonly<Record<string, string>>;

export type LabDraftResult =
  | { readonly ok: true; readonly value: LabCreateRequest }
  | { readonly ok: false; readonly errors: LabDraftErrors };

export type ConstraintParseResult =
  | { readonly ok: true; readonly constraints: readonly Constraint[] }
  | { readonly ok: false; readonly error: string };

export type TestParseResult =
  | { readonly ok: true; readonly tests: readonly TestItem[] }
  | { readonly ok: false; readonly error: string };

const TEST_FIELDS = new Set(["id", "input", "expectedOutput", "sample"]);
const DECIMAL_PATTERN = /^\d+(?:\.\d{1,2})?$/;
const textEncoder = new TextEncoder();

function utf8Length(value: string): number {
  return textEncoder.encode(value).length;
}

function codePointLength(value: string): number {
  return Array.from(value).length;
}

function isRecord(value: unknown): value is Readonly<Record<string, unknown>> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseConstraintLines(text: string): ConstraintParseResult {
  const lines = text.split(/\r?\n/);
  const constraints: Constraint[] = [];
  const seen = new Set<string>();

  for (let index = 0; index < lines.length; index += 1) {
    const line = (lines[index] ?? "").trim();
    if (line === "") continue;
    const lineNumber = index + 1;

    const firstSeparator = line.indexOf("|");
    const lastSeparator = line.lastIndexOf("|");
    if (firstSeparator === -1 || firstSeparator === lastSeparator) {
      return {
        ok: false,
        error: `Constraint line ${lineNumber}: use ID | text | deduction.`,
      };
    }

    const id = line.slice(0, firstSeparator).trim();
    const constraintText = line.slice(firstSeparator + 1, lastSeparator).trim();
    const deductionText = line.slice(lastSeparator + 1).trim();

    if (!LAB_BOUNDS.constraints.idPattern.test(id)) {
      return {
        ok: false,
        error: `Constraint line ${lineNumber}: ID must start with a letter, use [A-Za-z0-9_-], and be at most 32 characters.`,
      };
    }
    if (seen.has(id)) {
      return { ok: false, error: `Constraint line ${lineNumber}: ID "${id}" is duplicated.` };
    }
    seen.add(id);

    if (
      constraintText === "" ||
      codePointLength(constraintText) > LAB_BOUNDS.constraints.textMaxLength
    ) {
      return {
        ok: false,
        error: `Constraint line ${lineNumber}: text must be 1-${LAB_BOUNDS.constraints.textMaxLength} characters.`,
      };
    }

    if (!DECIMAL_PATTERN.test(deductionText)) {
      return {
        ok: false,
        error: `Constraint line ${lineNumber}: deduction must be a number with at most two decimal places.`,
      };
    }
    const deduction = Number.parseFloat(deductionText);
    if (
      deduction < LAB_BOUNDS.constraints.deductionMin ||
      deduction > LAB_BOUNDS.constraints.deductionMax
    ) {
      return {
        ok: false,
        error: `Constraint line ${lineNumber}: deduction must be between ${LAB_BOUNDS.constraints.deductionMin} and ${LAB_BOUNDS.constraints.deductionMax}.`,
      };
    }

    constraints.push({ id, text: constraintText, deduction });
  }

  if (constraints.length < LAB_BOUNDS.constraints.minItems) {
    return { ok: false, error: "Add at least one constraint." };
  }
  if (constraints.length > LAB_BOUNDS.constraints.maxItems) {
    return {
      ok: false,
      error: `At most ${LAB_BOUNDS.constraints.maxItems} constraints are allowed.`,
    };
  }
  return { ok: true, constraints };
}

export function parseTestItems(text: string): TestParseResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text) as unknown;
  } catch {
    return { ok: false, error: "Tests must be a valid JSON array." };
  }
  if (!Array.isArray(parsed)) {
    return { ok: false, error: "Tests must be a JSON array." };
  }
  if (parsed.length < LAB_BOUNDS.tests.minItems || parsed.length > LAB_BOUNDS.tests.maxItems) {
    return {
      ok: false,
      error: `Add between ${LAB_BOUNDS.tests.minItems} and ${LAB_BOUNDS.tests.maxItems} tests.`,
    };
  }

  const tests: TestItem[] = [];
  const ids = new Set<string>();
  let payloadBytes = 0;

  for (let index = 0; index < parsed.length; index += 1) {
    const item = parsed[index];
    const label = `Test ${index + 1}`;
    if (!isRecord(item)) {
      return { ok: false, error: `${label}: expected an object.` };
    }
    for (const key of Object.keys(item)) {
      if (!TEST_FIELDS.has(key)) {
        return { ok: false, error: `${label}: unknown field "${key}".` };
      }
    }

    const id = item["id"];
    const input = item["input"];
    const expectedOutput = item["expectedOutput"];
    const sample = item["sample"];

    if (typeof id !== "string" || id === "" || codePointLength(id) > LAB_BOUNDS.tests.idMaxLength) {
      return {
        ok: false,
        error: `${label}: "id" must be 1-${LAB_BOUNDS.tests.idMaxLength} characters.`,
      };
    }
    if (ids.has(id)) {
      return { ok: false, error: `${label}: "id" "${id}" is duplicated.` };
    }
    ids.add(id);
    if (typeof input !== "string" || typeof expectedOutput !== "string") {
      return { ok: false, error: `${label}: "input" and "expectedOutput" must be strings.` };
    }
    if (typeof sample !== "boolean") {
      return { ok: false, error: `${label}: "sample" must be true or false.` };
    }

    payloadBytes += utf8Length(input) + utf8Length(expectedOutput);
    if (payloadBytes > LAB_BOUNDS.tests.payloadMaxBytes) {
      return {
        ok: false,
        error: `Combined test payload must stay within ${LAB_BOUNDS.tests.payloadMaxBytes} bytes.`,
      };
    }

    tests.push({ id, input, expectedOutput, sample });
  }

  return { ok: true, tests };
}

function parseLimit(
  value: string,
  bounds: { readonly min: number; readonly max: number },
): number | null {
  if (value.trim() === "") return null;
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < bounds.min || parsed > bounds.max) return null;
  return parsed;
}

function toIsoTimestamp(value: string): string | null {
  if (value.trim() === "") return null;
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) return null;
  return timestamp.toISOString();
}

export function validateLabDraft(draft: LabDraftInput): LabDraftResult {
  const errors: Record<string, string> = {};

  const title = draft.title.trim();
  if (title === "" || codePointLength(title) > LAB_BOUNDS.title.maxLength) {
    errors["title"] = `Title must be 1-${LAB_BOUNDS.title.maxLength} characters.`;
  }

  const statement = draft.statement;
  if (statement.trim() === "" || codePointLength(statement) > LAB_BOUNDS.statement.maxLength) {
    errors["statement"] = `Statement must be 1-${LAB_BOUNDS.statement.maxLength} characters.`;
  }

  const opensAt = toIsoTimestamp(draft.opensAt);
  if (opensAt === null) {
    errors["opensAt"] = "Enter a valid opening date and time.";
  }
  const deadline = toIsoTimestamp(draft.deadline);
  if (deadline === null) {
    errors["deadline"] = "Enter a valid deadline date and time.";
  }
  if (opensAt !== null && deadline !== null && opensAt >= deadline) {
    errors["deadline"] = "The deadline must be strictly after the opening time.";
  }

  const constraintResult = parseConstraintLines(draft.constraintsText);
  if (!constraintResult.ok) {
    errors["constraints"] = constraintResult.error;
  }

  const testResult = parseTestItems(draft.testsText);
  if (!testResult.ok) {
    errors["tests"] = testResult.error;
  }

  const cpuSeconds = parseLimit(draft.cpuSeconds, LAB_BOUNDS.limits.cpuSeconds);
  if (cpuSeconds === null) {
    errors["cpuSeconds"] =
      `CPU seconds must be a whole number from ${LAB_BOUNDS.limits.cpuSeconds.min} to ${LAB_BOUNDS.limits.cpuSeconds.max}.`;
  }
  const memoryMiB = parseLimit(draft.memoryMiB, LAB_BOUNDS.limits.memoryMiB);
  if (memoryMiB === null) {
    errors["memoryMiB"] =
      `Memory must be a whole number from ${LAB_BOUNDS.limits.memoryMiB.min} to ${LAB_BOUNDS.limits.memoryMiB.max} MiB.`;
  }

  if (!constraintResult.ok || !testResult.ok) {
    return { ok: false, errors };
  }
  if (Object.keys(errors).length > 0) {
    return { ok: false, errors };
  }
  if (opensAt === null || deadline === null || cpuSeconds === null || memoryMiB === null) {
    // Unreachable: the branches above already recorded errors for nulls.
    return { ok: false, errors };
  }

  const limits: Limits = { cpuSeconds, memoryMiB };
  return {
    ok: true,
    value: {
      title,
      statement,
      opensAt,
      deadline,
      constraints: constraintResult.constraints,
      tests: testResult.tests,
      limits,
    },
  };
}
