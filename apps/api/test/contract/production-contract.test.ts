/**
 * Executable checks for the production contract documents (OpenSpec task 1.3).
 *
 * No production implementation exists yet. These tests keep the reviewed contract, its problem
 * catalog, the language-neutral fixtures, and the judge/checker envelopes consistent with each
 * other, so the fixtures can be bound to an implementation without first being repaired.
 * Expected values in the boundary tables are re-derived here from the rule stated in the
 * specification, never from the table itself.
 */
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { Ajv2020 } from "ajv/dist/2020.js";
import type { ValidateFunction } from "ajv/dist/2020.js";
import ajvFormats from "ajv-formats";
import { parse } from "yaml";

type Rec = Record<string, unknown>;

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "../../../..");
const contractRoot = join(repositoryRoot, "docs/contract");
const problemBase = "https://leetkob.example/problems/";
const methods = ["get", "put", "post", "patch", "delete"] as const;
const actorsVocabulary = [
  "anonymous",
  "admin",
  "ta-assigned",
  "ta-other-class",
  "ta-removed",
  "student-enrolled",
  "student-other",
  "student-not-enrolled",
];
const reviewedDecisions = ["D01", "D02", "D03", "D04", "D07", "D08", "D09"];

// ---- JSON navigation ---------------------------------------------------------------------------

function rec(value: unknown, label: string): Rec {
  assert.ok(
    typeof value === "object" && value !== null && !Array.isArray(value),
    `${label} must be an object`,
  );
  return value as Rec;
}

function list(value: unknown, label: string): unknown[] {
  assert.ok(Array.isArray(value), `${label} must be an array`);
  return value;
}

function text(value: unknown, label: string): string {
  assert.equal(typeof value, "string", `${label} must be a string`);
  return value as string;
}

function int(value: unknown, label: string): number {
  assert.ok(Number.isInteger(value), `${label} must be an integer`);
  return value as number;
}

function strings(value: unknown, label: string): string[] {
  return list(value, label).map((item, index) => text(item, `${label}[${index}]`));
}

function readJson(path: string): unknown {
  return JSON.parse(readFileSync(join(contractRoot, path), "utf8")) as unknown;
}

// ---- Documents ---------------------------------------------------------------------------------

const openapi = rec(parse(readFileSync(join(contractRoot, "v2/openapi.yaml"), "utf8")), "openapi");
const components = rec(openapi["components"], "components");
const componentSchemas = rec(components["schemas"], "components.schemas");
const catalog = rec(openapi["x-leetkob-problems"], "x-leetkob-problems");
const vectors = rec(readJson("v2/fixtures/vectors.json"), "vectors");
const messageSchema = rec(readJson("messages/v1/messages.schema.json"), "messages schema");
const messageFixtures = rec(readJson("messages/v1/fixtures/messages.json"), "message fixtures");

function deref(node: unknown, label: string): Rec {
  const object = rec(node, label);
  const reference = object["$ref"];
  if (typeof reference !== "string") return object;
  assert.ok(reference.startsWith("#/"), `${label}: only local references are allowed`);
  let current: unknown = openapi;
  for (const segment of reference.slice(2).split("/")) {
    current = rec(current, reference)[segment];
  }
  return deref(current, reference);
}

interface CatalogEntry {
  readonly status: number;
  readonly title: string;
  readonly schema: string;
  readonly universal: boolean;
  readonly sharedWithV1: boolean;
}

const problems = new Map<string, CatalogEntry>(
  Object.entries(catalog).map(([key, value]) => {
    const entry = rec(value, `problem ${key}`);
    return [
      key,
      {
        status: int(entry["status"], `${key}.status`),
        title: text(entry["title"], `${key}.title`),
        schema: text(entry["schema"], `${key}.schema`),
        universal: entry["universal"] === true,
        sharedWithV1: entry["sharedWithV1"] === true,
      },
    ];
  }),
);

interface Operation {
  readonly operationId: string;
  readonly method: string;
  readonly path: string;
  readonly pattern: RegExp;
  readonly raw: Rec;
  readonly meta: Rec;
  readonly problems: string[];
  readonly actors: string[];
  readonly parameters: Rec[];
  readonly responses: Rec;
}

const operations: Operation[] = [];
for (const [path, item] of Object.entries(rec(openapi["paths"], "paths"))) {
  const pathItem = rec(item, path);
  for (const method of methods) {
    if (pathItem[method] === undefined) continue;
    const raw = rec(pathItem[method], `${method} ${path}`);
    const label = `${method.toUpperCase()} ${path}`;
    const meta = rec(raw["x-leetkob"], `${label} x-leetkob`);
    operations.push({
      operationId: text(raw["operationId"], `${label} operationId`),
      method: method.toUpperCase(),
      path,
      pattern: new RegExp(`^${path.replaceAll(/\{[^}]+\}/g, "[^/?]+")}$`),
      raw,
      meta,
      problems: strings(meta["problems"], `${label} problems`),
      actors: strings(meta["actors"], `${label} actors`),
      parameters: list(raw["parameters"] ?? [], `${label} parameters`).map((parameter) =>
        deref(parameter, `${label} parameter`),
      ),
      responses: rec(raw["responses"], `${label} responses`),
    });
  }
}
const operationsById = new Map(operations.map((operation) => [operation.operationId, operation]));

// ---- Validators --------------------------------------------------------------------------------

// strictTypes and strictRequired are off because `if`/`then`/`anyOf` branches deliberately omit
// the type and properties of their parent schema.
const ajv = new Ajv2020({
  allErrors: true,
  strict: true,
  strictRequired: false,
  strictTypes: false,
});
ajvFormats.default(ajv);

const v2SchemaId = "https://leetkob.example/contract/v2";
ajv.addSchema({
  $id: v2SchemaId,
  $defs: JSON.parse(
    JSON.stringify(componentSchemas).replaceAll("#/components/schemas/", "#/$defs/"),
  ) as Rec,
});
ajv.addSchema(messageSchema);
const messageSchemaId = text(messageSchema["$id"], "messages $id");

function validator(reference: string): ValidateFunction {
  const found = ajv.getSchema(reference);
  assert.ok(found, `schema ${reference} must exist`);
  return found;
}

function v2Validator(name: string): ValidateFunction {
  return validator(`${v2SchemaId}#/$defs/${name}`);
}

function schemaName(schema: unknown, label: string): string {
  const reference = text(rec(schema, label)["$ref"], `${label} $ref`);
  const prefix = "#/components/schemas/";
  assert.ok(reference.startsWith(prefix), `${label} must reference a named schema`);
  return reference.slice(prefix.length);
}

function explain(validate: ValidateFunction): string {
  return JSON.stringify(validate.errors ?? []);
}

function header(headers: Rec, name: string): unknown {
  const key = Object.keys(headers).find((candidate) => candidate.toLowerCase() === name);
  return key === undefined ? undefined : headers[key];
}

// ---- Scenario fixtures -------------------------------------------------------------------------

interface Scenario {
  readonly id: string;
  readonly family: string;
  readonly operationId: string | null;
  readonly decision: string | null;
  readonly given: Rec;
  readonly request: Rec;
  readonly response: Rec;
  readonly requestSchemaValid: boolean;
}

const scenarioDirectory = join(contractRoot, "v2/fixtures/scenarios");
const scenarios: Scenario[] = readdirSync(scenarioDirectory)
  .filter((name) => name.endsWith(".json"))
  .sort()
  .flatMap((name) => {
    const file = rec(readJson(`v2/fixtures/scenarios/${name}`), name);
    const family = text(file["family"], `${name} family`);
    assert.equal(`${family}.json`, name, "a scenario file is named after its family");
    return list(file["scenarios"], `${name} scenarios`).map((value) => {
      const scenario = rec(value, `${name} scenario`);
      const id = text(scenario["id"], `${name} scenario id`);
      return {
        id,
        family,
        operationId:
          scenario["operationId"] === null ? null : text(scenario["operationId"], `${id} op`),
        decision:
          scenario["decision"] === undefined ? null : text(scenario["decision"], `${id} decision`),
        given: rec(scenario["given"], `${id} given`),
        request: rec(scenario["request"], `${id} request`),
        response: rec(scenario["response"], `${id} response`),
        requestSchemaValid: scenario["requestSchemaValid"] !== false,
      };
    });
  });

function problemCode(body: Rec, label: string): string {
  const type = text(body["type"], `${label} type`);
  assert.ok(
    type.startsWith(problemBase),
    `${label}: problem type must use the documented namespace`,
  );
  return type.slice(problemBase.length);
}

// ================================================================================================
// OpenAPI structure
// ================================================================================================

void test("every v2 operation is versioned, uniquely named, and fully annotated", () => {
  assert.match(text(openapi["openapi"], "openapi"), /^3\.1\./);
  assert.ok(operations.length > 0);
  assert.equal(operationsById.size, operations.length, "operationId values must be unique");

  for (const operation of operations) {
    const label = operation.operationId;
    assert.ok(operation.path.startsWith("/v2/"), `${label}: production paths live under /v2/`);
    assert.ok(!operation.path.endsWith("/"), `${label}: no trailing slash`);
    assert.equal(list(operation.raw["tags"], `${label} tags`).length, 1);

    const phase = int(operation.meta["phase"], `${label} phase`);
    assert.ok(phase >= 1 && phase <= 8, `${label}: phase 9 operations are not contracted yet`);
    assert.ok(operation.actors.length > 0);
    for (const actor of operation.actors) {
      assert.ok(["public", "admin", "ta", "student"].includes(actor), `${label}: actor ${actor}`);
    }
    assert.ok(
      [
        "none",
        "any",
        "own",
        "member-class",
        "assigned-class",
        "enrolled-class",
        "own-or-assigned-class",
      ].includes(text(operation.meta["scope"], `${label} scope`)),
      `${label}: unknown scope`,
    );
    assert.equal(new Set(operation.problems).size, operation.problems.length);
  }
});

void test("authentication, CSRF, and role problems follow from each operation's actors", () => {
  for (const operation of operations) {
    const label = operation.operationId;
    const isPublic = operation.actors.includes("public");
    const mutating = operation.method !== "GET";
    const csrf = text(operation.meta["csrf"], `${label} csrf`);
    const hasCsrfHeader = operation.parameters.some(
      (parameter) => parameter["name"] === "X-CSRF-Token" && parameter["in"] === "header",
    );

    if (isPublic) {
      assert.deepEqual(operation.actors, ["public"]);
      assert.deepEqual(operation.raw["security"], [], `${label}: public operations opt out`);
      assert.ok(!operation.problems.includes("authentication-required"));
    } else {
      assert.ok(
        operation.problems.includes("authentication-required"),
        `${label}: a protected operation documents authentication-required`,
      );
    }

    assert.equal(csrf === "none", !mutating, `${label}: exactly the mutations are CSRF-protected`);
    assert.equal(csrf === "token", hasCsrfHeader, `${label}: X-CSRF-Token matches csrf mode`);
    assert.equal(
      csrf === "token",
      operation.problems.includes("csrf-rejected"),
      `${label}: csrf-rejected is documented exactly for token-protected operations`,
    );
    if (csrf === "origin") {
      assert.ok(
        isPublic || label === "refreshSession",
        `${label}: only session-establishing operations rely on the Origin check alone`,
      );
    }

    const sorted = [...operation.actors].sort().join(",");
    const expectedRoleProblem = new Map([
      ["admin", "admin-role-required"],
      ["ta", "ta-role-required"],
      ["admin,ta", "ta-role-required"],
      ["student", "student-role-required"],
    ]).get(sorted);
    for (const roleProblem of [
      "admin-role-required",
      "ta-role-required",
      "student-role-required",
    ]) {
      assert.equal(
        operation.problems.includes(roleProblem),
        roleProblem === expectedRoleProblem,
        `${label}: ${roleProblem} must match actors [${sorted}]`,
      );
    }
  }
});

void test("idempotency, revision, and input metadata match the declared parameters", () => {
  const modes = new Map<string, number>();
  for (const operation of operations) {
    const label = operation.operationId;
    const mode = text(operation.meta["idempotency"], `${label} idempotency`);
    assert.ok(["none", "natural", "revision", "required"].includes(mode), `${label}: ${mode}`);
    modes.set(mode, (modes.get(mode) ?? 0) + 1);

    const keyParameter = operation.parameters.find(
      (parameter) => parameter["name"] === "Idempotency-Key",
    );
    assert.equal(mode === "required", keyParameter !== undefined, `${label}: Idempotency-Key`);
    if (keyParameter !== undefined) {
      assert.equal(keyParameter["in"], "header");
      assert.equal(keyParameter["required"], true);
    }
    assert.equal(
      mode === "revision",
      operation.problems.includes("revision-conflict"),
      `${label}: revision-conflict is documented exactly for revision-checked operations`,
    );

    const hasBody = operation.raw["requestBody"] !== undefined;
    assert.equal(
      hasBody,
      operation.problems.includes("unsupported-media-type"),
      `${label}: unsupported-media-type is documented exactly when a body is accepted`,
    );
    assert.equal(
      mode === "required" && hasBody,
      operation.problems.includes("idempotency-key-reused"),
      `${label}: a key can be reused with a different payload only when a payload exists`,
    );
    if (mode === "none") {
      assert.ok(
        operation.method === "GET" || label !== "publishLab",
        `${label}: publication always requires a key`,
      );
    }

    const pathParameters = [...operation.path.matchAll(/\{([^}]+)\}/g)].map((match) => match[1]);
    const declared = operation.parameters
      .filter((parameter) => parameter["in"] === "path")
      .map((parameter) => parameter["name"]);
    assert.deepEqual([...declared].sort(), [...pathParameters].sort(), `${label}: path parameters`);
    for (const parameter of operation.parameters) {
      assert.ok(
        ["path", "query", "header"].includes(text(parameter["in"], `${label} parameter in`)),
      );
      if (parameter["in"] === "path") assert.equal(parameter["required"], true);
    }
    if (operation.parameters.length > 0 || hasBody || mode === "required") {
      assert.ok(operation.problems.includes("invalid-request"), `${label}: invalid-request`);
    }
  }
  for (const mode of ["none", "natural", "revision", "required"]) {
    assert.ok((modes.get(mode) ?? 0) > 0, `at least one operation uses ${mode} idempotency`);
  }
  for (const required of ["createSubmission", "publishLab", "createAppeal"]) {
    assert.equal(operationsById.get(required)?.meta["idempotency"], "required", required);
  }
});

void test("documented responses are exactly the statuses of each operation's problems", () => {
  for (const operation of operations) {
    const label = operation.operationId;
    const expected = new Set<number>();
    for (const code of operation.problems) {
      const entry = problems.get(code);
      assert.ok(entry, `${label}: ${code} is not in the problem catalog`);
      assert.ok(
        !entry.universal,
        `${label}: universal problem ${code} is not listed per operation`,
      );
      expected.add(entry.status);
    }

    const documented = new Set<number>();
    let successes = 0;
    for (const [status, value] of Object.entries(operation.responses)) {
      const response = deref(value, `${label} ${status}`);
      const content = rec(response["content"] ?? {}, `${label} ${status} content`);
      if (status === "default" || Number(status) >= 400) {
        assert.deepEqual(
          Object.keys(content),
          ["application/problem+json"],
          `${label} ${status}: errors are RFC 9457 problem details`,
        );
        if (status !== "default") documented.add(Number(status));
      } else {
        successes += 1;
        assert.ok(Number(status) >= 200 && Number(status) < 300, `${label}: status ${status}`);
        assert.ok(!("application/problem+json" in content));
      }
      const headers = rec(response["headers"] ?? {}, `${label} ${status} headers`);
      assert.ok("Cache-Control" in headers, `${label} ${status}: Cache-Control: no-store`);
      if (status === "429") assert.ok("Retry-After" in headers, `${label}: Retry-After`);
    }
    assert.ok(successes >= 1, `${label}: at least one success response`);
    assert.ok("default" in operation.responses, `${label}: default covers universal problems`);
    assert.deepEqual(
      [...documented].sort(),
      [...expected].sort(),
      `${label}: error statuses must equal the statuses of its listed problems`,
    );
  }
});

// ================================================================================================
// Problem catalog
// ================================================================================================

function markdownTableRows(markdown: string): string[][] {
  return markdown
    .split("\n")
    .filter((line) => line.startsWith("|"))
    .map((line) =>
      line
        .split("|")
        .slice(1, -1)
        .map((cell) => cell.trim()),
    );
}

void test("the problem catalog is well formed and every type has a schema", () => {
  for (const [code, entry] of problems) {
    assert.match(code, /^[a-z][a-z0-9-]*$/);
    assert.ok(entry.status >= 400 && entry.status <= 599, `${code}: status`);
    assert.ok(entry.title.length > 0);
    assert.ok(entry.schema in componentSchemas, `${code}: schema ${entry.schema} must exist`);
    const sample = {
      type: `${problemBase}${code}`,
      title: entry.title,
      status: entry.status,
      detail: "A safe sentence.",
      instance: "/v2/example",
    };
    assert.ok(v2Validator("Problem")(sample), `${code}: base members satisfy Problem`);
  }
  for (const universal of ["route-not-found", "internal-error", "service-unavailable"]) {
    assert.equal(problems.get(universal)?.universal, true, universal);
  }
});

void test("problem types shared with v1.1 keep their status and title", () => {
  const v1 = readFileSync(join(contractRoot, "publish-lab.md"), "utf8");
  const v1Types = new Map<string, { status: number; title: string }>();
  for (const cells of markdownTableRows(v1)) {
    const [suffix, status, title] = cells;
    const match = /^`([a-z-]+)`$/.exec(suffix ?? "");
    if (match?.[1] === undefined || !/^\d{3}$/.test(status ?? "")) continue;
    v1Types.set(match[1], { status: Number(status), title: title ?? "" });
  }
  assert.equal(v1Types.size, 9, "the v1.1 contract documents nine problem types");

  for (const [code, expected] of v1Types) {
    const entry = problems.get(code);
    assert.ok(entry, `${code}: every v1.1 type remains defined in v2`);
    assert.equal(entry.sharedWithV1, true, `${code}: marked as shared`);
    assert.equal(entry.status, expected.status, `${code}: status is unchanged`);
    assert.equal(entry.title, expected.title, `${code}: title is unchanged`);
  }
  for (const [code, entry] of problems) {
    assert.equal(entry.sharedWithV1, v1Types.has(code), `${code}: sharedWithV1 flag`);
  }
});

void test("the v2 README problem table matches the catalog", () => {
  const readme = readFileSync(join(contractRoot, "v2/README.md"), "utf8");
  const documented = new Map<string, { status: number; title: string }>();
  for (const cells of markdownTableRows(readme)) {
    const [suffix, status, title] = cells;
    const match = /^`([a-z][a-z0-9-]*)`$/.exec(suffix ?? "");
    if (match?.[1] === undefined || !/^\d{3}$/.test(status ?? "")) continue;
    assert.ok(!documented.has(match[1]), `${match[1]}: listed once`);
    documented.set(match[1], { status: Number(status), title: title ?? "" });
  }
  assert.deepEqual([...documented.keys()].sort(), [...problems.keys()].sort());
  for (const [code, row] of documented) {
    assert.equal(row.status, problems.get(code)?.status, `${code}: status`);
    assert.equal(row.title, problems.get(code)?.title, `${code}: title`);
  }
});

// ================================================================================================
// Schemas and representations
// ================================================================================================

void test("request schemas are closed and carry no client-chosen authority", () => {
  for (const [name, schema] of Object.entries(componentSchemas)) {
    assert.ok(v2Validator(name), `${name} compiles under JSON Schema 2020-12`);
    if (!name.endsWith("Request")) continue;
    const object = rec(schema, name);
    assert.equal(object["additionalProperties"], false, `${name}: unknown members are rejected`);
    const properties = Object.keys(rec(object["properties"], `${name} properties`));
    for (const forbidden of ["role", "classIds", "grade", "deduction", "studentId", "userId"]) {
      if (name === "SetRoleRequest" && forbidden === "role") continue;
      if (name === "CreateConstraintRequest" && forbidden === "deduction") continue;
      if (name === "ReviseConstraintRequest" && forbidden === "deduction") continue;
      assert.ok(!properties.includes(forbidden), `${name}: must not accept ${forbidden}`);
    }
  }
  const register = v2Validator("RegisterRequest");
  const base = { email: "a@example.edu", password: "twelve chars!", displayName: "A" };
  assert.ok(register(base), explain(register));
  assert.ok(!register({ ...base, role: "ta" }), "registration cannot select a role");
  const decision = v2Validator("DecisionRequest");
  assert.ok(!decision({ expectedRevision: 1 }), "a decision has no default verdict");
  assert.ok(!decision({ expectedRevision: 1, verdict: "unsure" }), "unsure is not a decision");
  assert.ok(!decision({ expectedRevision: 1, verdict: "violation" }), "violation needs evidence");
  assert.ok(!decision({ expectedRevision: 1, verdict: "violation", evidence: [] }));
  assert.ok(decision({ expectedRevision: 1, verdict: "pass" }), explain(decision));
  assert.ok(!decision({ verdict: "pass" }), "a decision names the revision it reviewed");
  assert.ok(!decision({ expectedRevision: 1, verdict: "pass", deduction: "3.00" }));
});

void test("time, decimal, key, and identifier formats accept and reject the documented values", () => {
  for (const [name, value] of Object.entries(rec(vectors["formats"], "formats"))) {
    const cases = rec(value, name);
    const validate = v2Validator(name);
    for (const sample of strings(cases["valid"], `${name}.valid`)) {
      assert.ok(validate(sample), `${name} must accept ${JSON.stringify(sample)}`);
    }
    for (const sample of strings(cases["invalid"], `${name}.invalid`)) {
      assert.ok(!validate(sample), `${name} must reject ${JSON.stringify(sample)}`);
    }
  }
  const usd = v2Validator("MicroUsd");
  assert.ok(usd(250000));
  assert.ok(!usd(0.25), "money is whole micro-USD, never a fraction");
  assert.ok(!usd(-1));
});

void test("every success body uses the shared scalar schemas for time and points", () => {
  const serialized = JSON.stringify(componentSchemas);
  assert.ok(!serialized.includes('"format":"date"}'), "no date-only representation");
  for (const [name, schema] of Object.entries(componentSchemas)) {
    const properties = rec(rec(schema, name)["properties"] ?? {}, `${name} properties`);
    for (const [property, definition] of Object.entries(properties)) {
      const flat = JSON.stringify(definition);
      if (/(At|Deadline)$/.test(property)) {
        assert.ok(
          flat.includes("#/components/schemas/Timestamp"),
          `${name}.${property}: Timestamp`,
        );
      }
      if (/^(grade|formulaGrade|targetGrade|previousGrade|deduction)$/.test(property)) {
        assert.ok(flat.includes("#/components/schemas/Points"), `${name}.${property}: Points`);
      }
      if (/MicroUsd$/.test(property)) {
        assert.ok(flat.includes("#/components/schemas/MicroUsd"), `${name}.${property}: MicroUsd`);
      }
      if (/[Rr]evision$|^gradingGeneration$|^gradeVersion$/.test(property)) {
        assert.ok(flat.includes("#/components/schemas/Revision"), `${name}.${property}: Revision`);
      }
    }
  }
});

// ================================================================================================
// Boundary tables (expected values are re-derived from the stated rule)
// ================================================================================================

function cases(group: string): { readonly table: Rec; readonly rows: Rec[] } {
  const table = rec(vectors[group], group);
  return {
    table,
    rows: list(table["cases"], `${group}.cases`).map((row, index) =>
      rec(row, `${group}[${index}]`),
    ),
  };
}

function instant(value: unknown, label: string): number {
  const parsed = Date.parse(text(value, label));
  assert.ok(Number.isFinite(parsed), `${label} must be an instant`);
  return parsed;
}

function hundredths(value: unknown, label: string): number {
  const match = /^(-?)(\d+)\.(\d{2})$/.exec(text(value, label));
  assert.ok(match, `${label} must have two fractional digits`);
  const magnitude = Number(match[2]) * 100 + Number(match[3]);
  return match[1] === "-" ? -magnitude : magnitude;
}

function points(value: number): string {
  const sign = value < 0 ? "-" : "";
  const magnitude = Math.abs(value);
  return `${sign}${Math.floor(magnitude / 100)}.${String(magnitude % 100).padStart(2, "0")}`;
}

void test("execution limits use binary units and a wall time of three times CPU (D01, D04)", () => {
  const { table, rows } = cases("executionPolicy");
  const kibibyte = 1024;
  const mebibyte = 1024 * 1024;
  assert.deepEqual(table["fixed"], {
    sourceLimitBytes: 64 * kibibyte,
    compileTimeLimitMs: 10 * 1000,
    processLimit: 16,
    outputLimitBytes: mebibyte,
  });
  assert.deepEqual(table["defaults"], { cpuTimeLimitMs: 1000, memoryLimitBytes: 256 * mebibyte });

  const input = v2Validator("ExecutionPolicyInput");
  const full = v2Validator("ExecutionPolicy");
  const envelope = validator(`${messageSchemaId}#/$defs/ExecutionPolicy`);
  const fixed = rec(table["fixed"], "fixed");
  let accepted = 0;
  for (const row of rows) {
    const cpu = int(row["cpuTimeLimitMs"], "cpuTimeLimitMs");
    const memory = int(row["memoryLimitBytes"], "memoryLimitBytes");
    const valid =
      cpu >= 1000 &&
      cpu <= 5000 &&
      cpu % 1000 === 0 &&
      memory >= 64 * mebibyte &&
      memory <= 512 * mebibyte &&
      memory % mebibyte === 0;
    assert.equal(row["valid"], valid, `cpu ${cpu} ms, memory ${memory} bytes`);
    assert.equal(input({ cpuTimeLimitMs: cpu, memoryLimitBytes: memory }), valid, explain(input));

    const policy = {
      language: "cpp17",
      compileProfile: "gxx-std-cpp17-o2",
      ...fixed,
      cpuTimeLimitMs: cpu,
      wallTimeLimitMs: cpu * 3,
      memoryLimitBytes: memory,
    };
    assert.equal(full(policy), valid, `HTTP ExecutionPolicy: ${explain(full)}`);
    assert.equal(envelope(policy), valid, "the envelope policy agrees with the HTTP policy");
    if (valid) {
      accepted += 1;
      assert.equal(row["wallTimeLimitMs"], cpu * 3);
      assert.ok(!full({ ...policy, compileProfile: "gxx-std-cpp20-o3" }), "compiler is fixed");
      assert.ok(!full({ ...policy, outputLimitBytes: 1_000_000 }), "decimal megabyte is rejected");
      assert.ok(!full({ ...policy, sourceLimitBytes: 64_000 }), "decimal kilobyte is rejected");
      assert.ok(!full({ ...policy, processLimit: 17 }));
      assert.ok(!full({ ...policy, compileTimeLimitMs: 30_000 }));
    }
  }
  assert.ok(accepted >= 3 && accepted < rows.length, "both valid and invalid cases are present");
  assert.ok(!input({ compileProfile: "gxx-std-cpp17-o2" }), "the compiler cannot be supplied");
});

void test("size boundaries count bytes at exactly 65,536 and 1,048,576 (D01)", () => {
  const source = cases("sourceSize");
  const limit = int(source.table["limitBytes"], "limitBytes");
  assert.equal(limit, 2 ** 16);
  const sourceOutcomes = new Set<unknown>();
  for (const row of source.rows) {
    const bytes = int(row["bytes"], "bytes");
    const expected =
      bytes < 1 ? "invalid-request" : bytes <= limit ? "accepted" : "payload-too-large";
    assert.equal(row["outcome"], expected, `${bytes} source bytes`);
    sourceOutcomes.add(row["outcome"]);
  }
  assert.equal(sourceOutcomes.size, 3);
  for (const boundary of [limit - 1, limit, limit + 1]) {
    assert.ok(
      source.rows.some((row) => row["bytes"] === boundary),
      `source case ${boundary}`,
    );
  }

  const output = cases("outputSize");
  assert.equal(output.table["limitBytes"], 2 ** 20);
  for (const row of output.rows) {
    assert.equal(row["withinLimit"], int(row["bytes"], "bytes") <= 2 ** 20);
  }
  for (const boundary of [2 ** 20, 2 ** 20 + 1]) {
    assert.ok(
      output.rows.some((row) => row["bytes"] === boundary),
      `output case ${boundary}`,
    );
  }
});

void test("submission, publication, and appeal windows are half-open at their ends (D02, D03)", () => {
  const submission = cases("submissionWindow");
  const opens = instant(submission.table["opensAt"], "opensAt");
  const deadline = instant(submission.table["submissionDeadline"], "submissionDeadline");
  for (const row of submission.rows) {
    const received = instant(row["receivedAt"], "receivedAt");
    const expected =
      received < opens
        ? "lab-not-found"
        : received < deadline
          ? "accepted"
          : "submission-deadline-passed";
    assert.equal(row["outcome"], expected, text(row["receivedAt"], "receivedAt"));
  }
  const received = submission.rows.map((row) => instant(row["receivedAt"], "receivedAt"));
  for (const boundary of [opens - 1, opens, deadline - 1, deadline, deadline + 1]) {
    assert.ok(received.includes(boundary), "just before, at, and just after each boundary");
  }

  const publication = cases("publicationDeadline");
  const publishDeadline = instant(publication.table["submissionDeadline"], "deadline");
  assert.equal(publishDeadline, deadline, "both tables describe the same lab");
  for (const row of publication.rows) {
    const now = instant(row["serverTime"], "serverTime");
    assert.equal(row["outcome"], now >= publishDeadline ? "deadline-passed" : "lab-still-open");
  }
  const atDeadline = (rows: Rec[], key: string): unknown =>
    rows.find((row) => instant(row[key], key) === deadline)?.["outcome"];
  assert.equal(atDeadline(submission.rows, "receivedAt"), "submission-deadline-passed");
  assert.equal(
    atDeadline(publication.rows, "serverTime"),
    "deadline-passed",
    "at the deadline no submission is accepted and publication may proceed",
  );

  const appeal = cases("appealWindow");
  const published = instant(appeal.table["publishedAt"], "publishedAt");
  const end = published + 7 * 24 * 60 * 60 * 1000;
  assert.equal(instant(appeal.table["appealDeadline"], "appealDeadline"), end);
  for (const row of appeal.rows) {
    const now = instant(row["serverTime"], "serverTime");
    const eligible = now >= published && now < end;
    assert.equal(row["outcome"], eligible ? "eligible" : "appeal-window-closed");
  }
  const appealTimes = appeal.rows.map((row) => instant(row["serverTime"], "serverTime"));
  for (const boundary of [published, end - 1, end, end + 1]) {
    assert.ok(appealTimes.includes(boundary), "appeal boundaries are present");
  }
});

void test("the counted submission is the last received, whatever its verdict", () => {
  for (const row of cases("countedSelection").rows) {
    const submissions = list(row["submissions"], "submissions").map((value) => rec(value, "s"));
    const ordered = [...submissions].sort(
      (left, right) =>
        instant(left["receivedAt"], "receivedAt") - instant(right["receivedAt"], "receivedAt") ||
        int(left["acceptanceSequence"], "sequence") - int(right["acceptanceSequence"], "sequence"),
    );
    assert.equal(row["counted"], ordered.at(-1)?.["id"], text(row["id"], "id"));
  }
});

void test("pass sampling is the rounded-up tenth of the pass population", () => {
  const { rows } = cases("passSampling");
  for (const row of rows) {
    const population = int(row["passFindings"], "passFindings");
    assert.equal(row["requiredSample"], Math.ceil(population / 10), `P = ${population}`);
  }
  for (const population of [0, 1, 10, 11]) {
    assert.ok(
      rows.some((row) => row["passFindings"] === population),
      `P = ${population}`,
    );
  }
});

void test("grades deduct once per constraint, clamp to 0..10, and keep adjustments (D08)", () => {
  const { rows } = cases("grades");
  const seen = { clampedHigh: false, clampedLow: false, negative: false, replaced: false };
  for (const row of rows) {
    const id = text(row["id"], "grade case id");
    const accepted = row["judgeVerdict"] === "AC";
    const active = new Map<string, number>();
    for (const value of list(row["deductions"], `${id} deductions`)) {
      const deduction = rec(value, `${id} deduction`);
      const key = text(deduction["key"], "key");
      assert.ok(!active.has(key), `${id}: one deduction per constraint`);
      assert.ok(int(deduction["evidenceLines"], "evidenceLines") >= 1);
      active.set(key, hundredths(deduction["amount"], "amount"));
    }
    if (!accepted) assert.equal(active.size, 0, `${id}: non-Accepted work is never checked`);

    let adjustment: number | null = null;
    let adjustments = 0;
    const formula = (): number => {
      if (!accepted) return 0;
      let total = 0;
      for (const amount of active.values()) total += amount;
      return Math.max(0, 1000 - total);
    };
    const snapshot = (): Rec => {
      const raw = formula() + (adjustment ?? 0);
      if (raw > 1000) seen.clampedHigh = true;
      if (adjustment !== null && adjustment < 0) seen.negative = true;
      return {
        formulaGrade: points(formula()),
        manualAdjustmentAmount: adjustment === null ? null : points(adjustment),
        grade: points(Math.min(1000, Math.max(0, raw))),
      };
    };
    if (accepted && [...active.values()].reduce((sum, amount) => sum + amount, 0) > 1000) {
      seen.clampedLow = true;
    }

    const derived = [snapshot()];
    for (const value of list(row["steps"], `${id} steps`)) {
      const step = rec(value, `${id} step`);
      const operation = text(step["op"], "op");
      if (operation === "adjust") {
        adjustment = hundredths(step["targetGrade"], "targetGrade") - formula();
        adjustments += 1;
        if (adjustments > 1) seen.replaced = true;
      } else if (operation === "appeal-accepted") {
        assert.ok(active.delete(text(step["key"], "key")), `${id}: appealed deduction exists`);
      } else {
        assert.equal(operation, "appeal-rejected");
        assert.ok(active.has(text(step["key"], "key")));
      }
      derived.push(snapshot());
    }
    assert.deepEqual(row["expected"], derived, id);
    for (const state of derived) {
      assert.ok(v2Validator("Points")(state["grade"]), `${id}: grade is canonical`);
      assert.ok(v2Validator("Points")(state["formulaGrade"]));
      if (state["manualAdjustmentAmount"] !== null) {
        assert.ok(v2Validator("SignedPoints")(state["manualAdjustmentAmount"]));
      }
    }
  }
  assert.deepEqual(seen, { clampedHigh: true, clampedLow: true, negative: true, replaced: true });
});

void test("evidence matches after trimming outer whitespace only (D09)", () => {
  const { rows } = cases("evidence");
  const outcomes = new Set<unknown>();
  for (const row of rows) {
    const original = text(row["originalLine"], "originalLine");
    const cited = text(row["citedText"], "citedText");
    assert.equal(row["matches"], original.trim() === cited.trim(), JSON.stringify(cited));
    outcomes.add(row["matches"]);
  }
  assert.deepEqual([...outcomes].sort(), [false, true]);
  assert.ok(
    rows.some(
      (row) =>
        row["matches"] === false &&
        text(row["originalLine"], "o").replaceAll(/\s/g, "") ===
          text(row["citedText"], "c").replaceAll(/\s/g, ""),
    ),
    "an inner-whitespace difference is a mismatch",
  );
});

void test("ingestion and class limits reject the first value above each ceiling (D07)", () => {
  const bundle = cases("testBundleLimits");
  const limits = rec(bundle.table["limits"], "limits");
  assert.deepEqual(limits, {
    compressedBytes: 10 * 1024 * 1024,
    expandedBytes: 100 * 1024 * 1024,
    entries: 1000,
  });
  for (const row of bundle.rows) {
    const compressed = int(row["compressedBytes"], "compressedBytes");
    const violations: string[] = [];
    if (int(row["entries"], "entries") > 1000) violations.push("entry-limit-exceeded");
    if (int(row["expandedBytes"], "expandedBytes") > 100 * 1024 * 1024) {
      violations.push("expanded-size-exceeded");
    }
    if (compressed > 10 * 1024 * 1024) {
      assert.equal(row["outcome"], "payload-too-large");
    } else if (violations.length > 0) {
      assert.equal(row["outcome"], "test-bundle-invalid");
      assert.deepEqual(row["violations"], violations);
    } else {
      assert.equal(row["outcome"], "accepted");
    }
  }
  const revision = rec(rec(componentSchemas["TestRevision"], "TestRevision")["properties"], "p");
  assert.equal(rec(revision["compressedBytes"], "c")["maximum"], limits["compressedBytes"]);
  assert.equal(rec(revision["expandedBytes"], "e")["maximum"], limits["expandedBytes"]);

  const capacity = cases("classCapacity");
  assert.equal(capacity.table["capacity"], 100);
  for (const row of capacity.rows) {
    const before = int(row["studentsBefore"], "studentsBefore");
    assert.equal(row["outcome"], before < 100 ? "enrolled" : "class-capacity-reached");
    assert.equal(row["studentsAfter"], Math.min(100, before + 1));
  }
  assert.ok(capacity.rows.some((row) => row["studentsBefore"] === 99));
  assert.ok(capacity.rows.some((row) => row["studentsBefore"] === 100));
});

function aggregate(compile: unknown, tests: unknown[]): unknown {
  if (compile !== "ok") return "CE";
  return tests.find((verdict) => verdict !== "AC") ?? "AC";
}

void test("the aggregate verdict is CE or the first non-AC test in stable order", () => {
  const verdicts = new Set<unknown>();
  for (const row of cases("aggregateVerdict").rows) {
    const tests = list(row["tests"], "tests");
    assert.equal(row["verdict"], aggregate(row["compile"], tests));
    if (row["compile"] !== "ok") assert.equal(tests.length, 0, "no test runs after CE");
    verdicts.add(row["verdict"]);
  }
  assert.deepEqual([...verdicts].sort(), ["AC", "CE", "MLE", "RE", "TLE", "WA"]);
});

void test("constraint edits follow the lifecycle decision table", () => {
  const { rows } = cases("constraintEdits");
  const rules = new Map([
    ["add", "addition-after-opening"],
    ["increase-deduction", "deduction-increase-after-opening"],
    ["change-text", "meaning-not-confirmed"],
  ]);
  for (const row of rows) {
    const phase = text(row["phase"], "phase");
    const change = text(row["change"], "change");
    const label = `${phase} / ${change}`;
    if (phase === "published") {
      assert.equal(row["outcome"], "lab-already-published", label);
    } else if (phase === "before-opening" || !rules.has(change)) {
      assert.equal(row["outcome"], "allowed", label);
    } else {
      assert.equal(row["outcome"], "constraint-change-not-allowed", label);
      assert.equal(row["rule"], rules.get(change), label);
    }
  }
  const schema = rec(componentSchemas["ConstraintChangeNotAllowedProblem"], "schema");
  const documented = JSON.stringify(schema);
  for (const rule of rules.values()) assert.ok(documented.includes(rule), rule);
});

// ================================================================================================
// Scenario fixtures
// ================================================================================================

void test("every scenario is a valid exchange of its operation", () => {
  const ids = new Set<string>();
  for (const scenario of scenarios) {
    const { id } = scenario;
    assert.ok(!ids.has(id), `${id}: scenario ids are unique`);
    ids.add(id);
    assert.ok(id.startsWith(`${scenario.family}.`), `${id}: prefixed by its family`);
    assert.ok(v2Validator("Timestamp")(scenario.given["now"]), `${id}: given.now`);
    assert.ok(actorsVocabulary.includes(text(scenario.given["actor"], `${id} actor`)), id);
    if (scenario.decision !== null) assert.ok(reviewedDecisions.includes(scenario.decision), id);

    const method = text(scenario.request["method"], `${id} method`);
    const path = text(scenario.request["path"], `${id} path`);
    const pathOnly = path.split("?")[0] ?? path;
    const headers = rec(scenario.request["headers"] ?? {}, `${id} headers`);
    const status = int(scenario.response["status"], `${id} status`);
    const contentType = scenario.response["contentType"];

    if (scenario.operationId === null) {
      assert.ok(
        !operations.some(
          (candidate) => candidate.method === method && candidate.pattern.test(pathOnly),
        ),
        `${id}: must not match any documented operation`,
      );
      const body = rec(scenario.response["body"], `${id} body`);
      assert.equal(problemCode(body, id), "route-not-found");
    }
    const operation =
      scenario.operationId === null ? undefined : operationsById.get(scenario.operationId);
    if (scenario.operationId !== null) {
      assert.ok(operation, `${id}: unknown operation ${scenario.operationId}`);
      assert.equal(method, operation.method, `${id}: method`);
      assert.match(pathOnly, operation.pattern, `${id}: path`);
    }

    // Request.
    const key = header(headers, "idempotency-key");
    if (key !== undefined) {
      assert.ok(v2Validator("IdempotencyKey")(key), `${id}: Idempotency-Key`);
      if (operation !== undefined) {
        assert.equal(operation.meta["idempotency"], "required", `${id}: key on a keyed operation`);
      }
    }
    assert.equal(header(headers, "authorization"), undefined, `${id}: v2 uses the session cookie`);
    if ("body" in scenario.request) {
      assert.ok(operation, id);
      const content = rec(
        deref(operation.raw["requestBody"], `${id} requestBody`)["content"],
        `${id} content`,
      );
      const validate = v2Validator(
        schemaName(rec(content["application/json"], `${id} json`)["schema"], `${id} schema`),
      );
      assert.equal(
        validate(scenario.request["body"]),
        scenario.requestSchemaValid,
        `${id}: request body validity. ${explain(validate)}`,
      );
    } else {
      assert.ok(scenario.requestSchemaValid, `${id}: requestSchemaValid needs a JSON body`);
    }

    // Response.
    if (contentType === "application/problem+json") {
      const body = rec(scenario.response["body"], `${id} body`);
      const code = problemCode(body, id);
      const entry = problems.get(code);
      assert.ok(entry, `${id}: ${code} is not in the catalog`);
      assert.equal(status, entry.status, `${id}: HTTP status of ${code}`);
      assert.equal(body["status"], entry.status, `${id}: status member`);
      assert.equal(body["title"], entry.title, `${id}: title of ${code}`);
      assert.equal(body["instance"], pathOnly, `${id}: instance is the request path`);
      const validate = v2Validator(entry.schema);
      assert.ok(validate(body), `${id}: ${entry.schema}. ${explain(validate)}`);
      if (operation !== undefined) {
        assert.ok(
          entry.universal || operation.problems.includes(code),
          `${id}: ${operation.operationId} does not document ${code}`,
        );
      }
      if (code.endsWith("-not-found")) {
        assert.deepEqual(
          Object.keys(body).sort(),
          ["detail", "instance", "status", "title", "type"],
          `${id}: a concealed not-found never carries extension members`,
        );
      }
      if (code === "rate-limited") {
        const responseHeaders = rec(scenario.response["headers"], `${id} response headers`);
        assert.equal(responseHeaders["Retry-After"], String(body["retryAfterSeconds"]), id);
      }
      if (!scenario.requestSchemaValid) assert.equal(code, "invalid-request", id);
    } else {
      assert.ok(operation, id);
      assert.ok(status >= 200 && status < 300, `${id}: status ${status}`);
      const response = deref(operation.responses[String(status)], `${id} response ${status}`);
      const content = rec(response["content"] ?? {}, `${id} response content`);
      if (contentType === null) {
        assert.equal(status, 204, id);
        assert.equal(scenario.response["body"], null, id);
        assert.deepEqual(Object.keys(content), [], id);
      } else if (contentType === "application/json") {
        const validate = v2Validator(
          schemaName(rec(content["application/json"], `${id} json`)["schema"], `${id} schema`),
        );
        assert.ok(validate(scenario.response["body"]), `${id}: body. ${explain(validate)}`);
      } else {
        assert.ok(
          text(contentType, `${id} contentType`) in content,
          `${id}: ${String(contentType)}`,
        );
        text(scenario.response["bodyText"], `${id} bodyText`);
      }
    }

    if ("events" in scenario.response) {
      const stream = text(scenario.response["bodyText"], `${id} stream`);
      const validate = v2Validator("EventData");
      for (const value of list(scenario.response["events"], `${id} events`)) {
        const event = rec(value, `${id} event`);
        assert.ok(validate(event), `${id}: EventData. ${explain(validate)}`);
        const frame = `id: ${String(event["eventId"])}\nevent: ${String(event["type"])}\ndata: ${JSON.stringify(event)}\n\n`;
        assert.ok(stream.includes(frame), `${id}: stream frames carry id, event, and data`);
      }
    }
  }
});

void test("scenarios cover every operation, every problem type, and every reviewed decision", () => {
  const coveredOperations = new Set(scenarios.map((scenario) => scenario.operationId));
  for (const operation of operations) {
    assert.ok(
      coveredOperations.has(operation.operationId),
      `${operation.operationId}: no scenario`,
    );
  }

  const coveredProblems = new Set<string>();
  const perOperation = new Map<string, Set<string>>();
  for (const scenario of scenarios) {
    if (scenario.response["contentType"] !== "application/problem+json") continue;
    const code = problemCode(rec(scenario.response["body"], scenario.id), scenario.id);
    coveredProblems.add(code);
    if (scenario.operationId !== null) {
      const set = perOperation.get(scenario.operationId) ?? new Set<string>();
      set.add(code);
      perOperation.set(scenario.operationId, set);
    }
  }
  assert.deepEqual([...coveredProblems].sort(), [...problems.keys()].sort());

  // The irreversible and deadline-bound operations get every documented problem exercised.
  for (const critical of ["publishLab", "createSubmission", "createAppeal"]) {
    const operation = operationsById.get(critical);
    assert.ok(operation, critical);
    const exercised = perOperation.get(critical) ?? new Set<string>();
    const optional = new Set([
      "authentication-required",
      "csrf-rejected",
      "unsupported-media-type",
      "rate-limited",
    ]);
    for (const code of operation.problems) {
      if (optional.has(code) || code.endsWith("-role-required")) continue;
      assert.ok(exercised.has(code), `${critical}: no scenario for ${code}`);
    }
  }
  for (const code of ["authentication-required", "ta-role-required", "internal-error"]) {
    assert.ok(perOperation.get("publishLab")?.has(code), `publishLab: no scenario for ${code}`);
  }

  const decisions = new Set<string>();
  for (const scenario of scenarios)
    if (scenario.decision !== null) decisions.add(scenario.decision);
  for (const value of Object.values(vectors)) {
    if (typeof value !== "object" || value === null) continue;
    for (const decision of strings(rec(value, "vector")["decision"] ?? [], "decision")) {
      decisions.add(decision);
    }
  }
  assert.deepEqual([...decisions].sort(), reviewedDecisions);
});

void test("a repeated key with the same actor and payload always replays the same response", () => {
  const groups = new Map<string, Scenario[]>();
  for (const scenario of scenarios) {
    const headers = rec(scenario.request["headers"] ?? {}, `${scenario.id} headers`);
    const key = header(headers, "idempotency-key");
    if (key === undefined || scenario.operationId === null) continue;
    const payload = JSON.stringify([
      scenario.request["body"] ?? null,
      scenario.request["bodyText"] ?? null,
      scenario.request["bodyRepeat"] ?? null,
    ]);
    const group = [
      scenario.operationId,
      scenario.request["path"],
      scenario.given["actor"],
      key,
      payload,
    ].join("\n");
    groups.set(group, [...(groups.get(group) ?? []), scenario]);
  }

  const replayed = new Set<string>();
  const replayedRejection = new Set<string>();
  for (const group of groups.values()) {
    const [first, ...rest] = group;
    assert.ok(first);
    for (const other of rest) {
      assert.deepEqual(
        other.response,
        first.response,
        `${other.id} must replay the stored response of ${first.id}`,
      );
      assert.notEqual(other.given["now"], first.given["now"], `${other.id}: a later retry`);
      replayed.add(text(first.operationId, "operationId"));
      if (int(first.response["status"], "status") === 409) {
        replayedRejection.add(text(first.operationId, "operationId"));
      }
    }
  }
  assert.deepEqual([...replayed].sort(), ["createAppeal", "createSubmission", "publishLab"]);
  assert.ok(replayedRejection.has("publishLab"), "a deterministic rejection is replayed too");

  // The same key with a different payload or a revoked actor is never a replay.
  const byId = new Map(scenarios.map((scenario) => [scenario.id, scenario]));
  assert.equal(byId.get("submissions.create.key-reused")?.response["status"], 409);
  assert.equal(byId.get("appeals.create.key-reused")?.response["status"], 409);
  assert.equal(byId.get("publication.former-ta-replay")?.response["status"], 404);
});

void test("the unversioned v1.1 route is not part of the production contract", () => {
  assert.ok(!operations.some((operation) => operation.path === "/labs/{labId}/publish"));
  const legacy = scenarios.find((scenario) => scenario.id === "publication.legacy-route-absent");
  assert.ok(legacy);
  assert.equal(legacy.request["path"], "/labs/1ab00000-0000-4000-8000-000000000003/publish");
  assert.equal(legacy.response["status"], 404);

  const publication = rec(componentSchemas["Publication"], "Publication");
  assert.equal(publication["additionalProperties"], false);
  assert.deepEqual(Object.keys(rec(publication["properties"], "properties")).sort(), [
    "labId",
    "publishedAt",
    "publishedBy",
    "status",
  ]);
  const publish = operationsById.get("publishLab");
  assert.ok(publish);
  assert.equal(publish.raw["requestBody"], undefined, "publication has an empty body");
  assert.deepEqual([...publish.problems].sort(), [
    "authentication-required",
    "csrf-rejected",
    "invalid-request",
    "lab-already-published",
    "lab-not-found",
    "lab-processing-incomplete",
    "lab-still-open",
    "reviews-pending",
    "ta-role-required",
  ]);
});

// ================================================================================================
// Judge and checker envelopes
// ================================================================================================

void test("envelope fixtures are accepted or rejected by the versioned schema", () => {
  const validate = validator(messageSchemaId);
  const types = new Set<unknown>();
  for (const value of list(messageFixtures["valid"], "valid messages")) {
    const fixture = rec(value, "valid message");
    const id = text(fixture["id"], "id");
    const message = rec(fixture["message"], id);
    assert.ok(validate(message), `${id} must be valid. ${explain(validate)}`);
    types.add(message["type"]);

    const payload = rec(message["payload"], `${id} payload`);
    if (message["type"] === "leetkob.judge.result" && payload["outcome"] === "completed") {
      const verdicts = list(payload["tests"], "tests").map(
        (entry) => rec(entry, "test")["verdict"],
      );
      const compile = rec(payload["compile"], "compile")["status"];
      assert.equal(payload["verdict"], aggregate(compile, verdicts), `${id}: aggregate verdict`);
    }
    if (message["type"] === "leetkob.check.result" && payload["outcome"] === "completed") {
      const keys = list(payload["findings"], "findings").map(
        (f) => rec(f, "finding")["constraintId"],
      );
      assert.equal(new Set(keys).size, keys.length, `${id}: one finding per constraint`);
    }
    const source = payload["source"];
    if (source !== undefined) {
      const encoded = rec(source, `${id} source`);
      const decoded = Buffer.from(text(encoded["data"], "data"), "base64");
      assert.equal(encoded["bytes"], decoded.length, `${id}: bytes is the decoded length`);
    }
  }
  assert.deepEqual([...types].sort(), [
    "leetkob.check.request",
    "leetkob.check.result",
    "leetkob.judge.request",
    "leetkob.judge.result",
  ]);

  const invalidIds = new Set<string>();
  for (const value of list(messageFixtures["invalid"], "invalid messages")) {
    const fixture = rec(value, "invalid message");
    const id = text(fixture["id"], "id");
    assert.ok(text(fixture["reason"], `${id} reason`).length > 0);
    assert.ok(!validate(fixture["message"]), `${id} must be rejected`);
    invalidIds.add(id);
  }
  for (const guardrail of [
    "unsupported-major-version",
    "judge-request-with-credential-member",
    "check-request-with-deduction",
    "check-result-with-deduction",
    "check-result-violation-without-evidence",
    "check-result-unsure-without-cause",
  ]) {
    assert.ok(invalidIds.has(guardrail), `guardrail fixture ${guardrail}`);
  }
});

void test("the checker result and the HTTP contract share one verdict vocabulary", () => {
  const finding = rec(rec(messageSchema["$defs"], "$defs")["Finding"], "Finding");
  const properties = rec(finding["properties"], "Finding properties");
  const httpCauses = strings(rec(componentSchemas["UnsureCause"], "UnsureCause")["enum"], "causes");
  const envelopeCauses = list(rec(properties["unsureCause"], "unsureCause")["enum"], "enum");
  assert.deepEqual(envelopeCauses, [null, ...httpCauses]);
  assert.deepEqual(
    rec(properties["verdict"], "verdict")["enum"],
    rec(componentSchemas["CheckVerdict"], "CheckVerdict")["enum"],
  );
  assert.deepEqual(
    rec(properties["source"], "source")["enum"],
    rec(componentSchemas["FindingSource"], "FindingSource")["enum"],
  );
  for (const forbidden of ["grade", "score", "deduction", "amount"]) {
    assert.ok(!(forbidden in properties), `a finding has no ${forbidden} member`);
  }
  assert.equal(finding["additionalProperties"], false);
});

void test("prediction lines follow the official evaluation format", () => {
  const validate = validator(`${messageSchemaId}#/$defs/Prediction`);
  const predictions = rec(messageFixtures["predictions"], "predictions");
  for (const line of list(predictions["valid"], "valid predictions")) {
    assert.ok(validate(line), explain(validate));
  }
  for (const value of list(predictions["invalid"], "invalid predictions")) {
    assert.ok(!validate(rec(value, "invalid prediction")["line"]));
  }
});
