/**
 * Hand-derived PA#3 API types and runtime parsers.
 *
 * Source of truth: `docs/contract/pa3-openapi.json` (OpenAPI 3.1) for exactly
 * the routes the web application uses:
 *
 * - `POST /v2/auth/login`
 * - `GET /v2/auth/me`
 * - `POST /v2/auth/logout`
 * - `GET /v2/classes`
 * - `GET|POST /v2/classes/{classId}/labs`
 * - `GET /v2/labs/{labId}`
 *
 * The parsers reject responses that drift from the contract instead of
 * rendering unverified data. `scripts/openapi-drift-check.mjs` re-checks the
 * declared route set, fields, bounds and enums against the OpenAPI document.
 */

export const API_ROUTES = {
  login: "/v2/auth/login",
  me: "/v2/auth/me",
  logout: "/v2/auth/logout",
  classes: "/v2/classes",
  classLabs: (classId: string) => `/v2/classes/${encodeURIComponent(classId)}/labs`,
  lab: (labId: string) => `/v2/labs/${encodeURIComponent(labId)}`,
} as const;

export const ROLES = ["admin", "ta", "student"] as const;

export type Role = (typeof ROLES)[number];

export interface User {
  readonly id: string;
  readonly displayName: string;
  readonly role: Role;
}

export interface ClassSummary {
  readonly id: string;
  readonly name: string;
}

export interface UserSession {
  readonly user: User;
  readonly csrfToken: string;
  readonly expiresAt: string;
}

export interface CurrentUser {
  readonly user: User;
  readonly classes: readonly ClassSummary[];
}

export interface Paginated<T> {
  readonly items: readonly T[];
  readonly nextCursor: string | null;
}

export interface Constraint {
  readonly id: string;
  readonly text: string;
  readonly deduction: number;
}

export interface TestItem {
  readonly id: string;
  readonly input: string;
  readonly expectedOutput: string;
  readonly sample: boolean;
}

export interface Limits {
  readonly cpuSeconds: number;
  readonly memoryMiB: number;
}

export interface LabCreateRequest {
  readonly title: string;
  readonly statement: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly constraints: readonly Constraint[];
  readonly tests: readonly TestItem[];
  readonly limits: Limits;
}

export interface Lab {
  readonly id: string;
  readonly classId: string;
  readonly title: string;
  readonly statement: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly revision: number;
  readonly constraints: readonly Constraint[];
  readonly limits: Limits;
  readonly modelNotice: string;
}

export interface LabSummary {
  readonly id: string;
  readonly classId: string;
  readonly title: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly revision: number;
}

export interface LabDetail extends Lab {
  readonly sampleTests: readonly TestItem[];
}

type JsonRecord = Readonly<Record<string, unknown>>;

function asRecord(value: unknown, context: string): JsonRecord {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${context}: expected an object`);
  }
  return value as JsonRecord;
}

function asString(record: JsonRecord, key: string, context: string): string {
  const value = record[key];
  if (typeof value !== "string") {
    throw new Error(`${context}: "${key}" must be a string`);
  }
  return value;
}

function asInteger(record: JsonRecord, key: string, context: string): number {
  const value = record[key];
  if (typeof value !== "number" || !Number.isInteger(value)) {
    throw new Error(`${context}: "${key}" must be an integer`);
  }
  return value;
}

function asBoolean(record: JsonRecord, key: string, context: string): boolean {
  const value = record[key];
  if (typeof value !== "boolean") {
    throw new Error(`${context}: "${key}" must be a boolean`);
  }
  return value;
}

function asArray(record: JsonRecord, key: string, context: string): readonly unknown[] {
  const value = record[key];
  if (!Array.isArray(value)) {
    throw new Error(`${context}: "${key}" must be an array`);
  }
  return value;
}

function parseRole(value: unknown, context: string): Role {
  if (typeof value === "string" && (ROLES as readonly string[]).includes(value)) {
    return value as Role;
  }
  throw new Error(`${context}: role must be one of ${ROLES.join(", ")}`);
}

export function parseUser(value: unknown, context = "user"): User {
  const record = asRecord(value, context);
  return {
    id: asString(record, "id", context),
    displayName: asString(record, "displayName", context),
    role: parseRole(record["role"], context),
  };
}

export function parseClassSummary(value: unknown, context = "class"): ClassSummary {
  const record = asRecord(value, context);
  return {
    id: asString(record, "id", context),
    name: asString(record, "name", context),
  };
}

export function parseUserSession(value: unknown): UserSession {
  const record = asRecord(value, "session");
  return {
    user: parseUser(record["user"], "session.user"),
    csrfToken: asString(record, "csrfToken", "session"),
    expiresAt: asString(record, "expiresAt", "session"),
  };
}

export function parseCurrentUser(value: unknown): CurrentUser {
  const record = asRecord(value, "current-user");
  return {
    user: parseUser(record["user"], "current-user.user"),
    classes: asArray(record, "classes", "current-user").map((item) =>
      parseClassSummary(item, "current-user.classes[]"),
    ),
  };
}

export function parseClassList(value: unknown): Paginated<ClassSummary> {
  const record = asRecord(value, "class-list");
  return {
    items: asArray(record, "items", "class-list").map((item) =>
      parseClassSummary(item, "class-list.items[]"),
    ),
    nextCursor: parseCursor(record, "class-list"),
  };
}

function parseCursor(record: JsonRecord, context: string): string | null {
  const value = record["nextCursor"];
  if (value === null) return null;
  if (typeof value === "string") return value;
  throw new Error(`${context}: "nextCursor" must be a string or null`);
}

export function parseConstraint(value: unknown, context = "constraint"): Constraint {
  const record = asRecord(value, context);
  const deduction = record["deduction"];
  if (typeof deduction !== "number" || !Number.isFinite(deduction)) {
    throw new Error(`${context}: "deduction" must be a number`);
  }
  return {
    id: asString(record, "id", context),
    text: asString(record, "text", context),
    deduction,
  };
}

export function parseTestItem(value: unknown, context = "test"): TestItem {
  const record = asRecord(value, context);
  return {
    id: asString(record, "id", context),
    input: asString(record, "input", context),
    expectedOutput: asString(record, "expectedOutput", context),
    sample: asBoolean(record, "sample", context),
  };
}

export function parseLimits(value: unknown, context = "limits"): Limits {
  const record = asRecord(value, context);
  return {
    cpuSeconds: asInteger(record, "cpuSeconds", context),
    memoryMiB: asInteger(record, "memoryMiB", context),
  };
}

export function parseLab(value: unknown): Lab {
  const record = asRecord(value, "lab");
  return {
    id: asString(record, "id", "lab"),
    classId: asString(record, "classId", "lab"),
    title: asString(record, "title", "lab"),
    statement: asString(record, "statement", "lab"),
    opensAt: asString(record, "opensAt", "lab"),
    deadline: asString(record, "deadline", "lab"),
    revision: asInteger(record, "revision", "lab"),
    constraints: asArray(record, "constraints", "lab").map((item) =>
      parseConstraint(item, "lab.constraints[]"),
    ),
    limits: parseLimits(record["limits"], "lab.limits"),
    modelNotice: asString(record, "modelNotice", "lab"),
  };
}

export function parseLabSummaryList(value: unknown): Paginated<LabSummary> {
  const record = asRecord(value, "lab-list");
  return {
    items: asArray(record, "items", "lab-list").map((item) => {
      const lab = asRecord(item, "lab-list.items[]");
      return {
        id: asString(lab, "id", "lab-list.items[]"),
        classId: asString(lab, "classId", "lab-list.items[]"),
        title: asString(lab, "title", "lab-list.items[]"),
        opensAt: asString(lab, "opensAt", "lab-list.items[]"),
        deadline: asString(lab, "deadline", "lab-list.items[]"),
        revision: asInteger(lab, "revision", "lab-list.items[]"),
      };
    }),
    nextCursor: parseCursor(record, "lab-list"),
  };
}

export function parseLabDetail(value: unknown): LabDetail {
  const record = asRecord(value, "lab-detail");
  const lab = parseLab(record);
  return {
    ...lab,
    sampleTests: asArray(record, "sampleTests", "lab-detail").map((item) =>
      parseTestItem(item, "lab-detail.sampleTests[]"),
    ),
  };
}
