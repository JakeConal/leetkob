#!/usr/bin/env node
/* global process */
/**
 * Contract drift check for the hand-derived web API types.
 *
 * `apps/web/lib/api-types.ts` is written by hand from
 * `docs/contract/pa3-openapi.json`. This script fails (exit 1) when a route,
 * request/response schema, required field, type, bound, enum or parameter the
 * web client relies on no longer matches the published OpenAPI document, so a
 * contract change cannot silently break the screens.
 *
 * Usage:
 *   node scripts/openapi-drift-check.mjs
 *   node scripts/openapi-drift-check.mjs --openapi /tmp/pa3-openapi.json
 */

import { readFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

export const DEFAULT_OPENAPI_PATH = path.join(
  import.meta.dirname,
  "..",
  "docs",
  "contract",
  "pa3-openapi.json",
);

function writeStderr(line) {
  process.stderr.write(`${line}\n`);
}

function writeStdout(line) {
  process.stdout.write(`${line}\n`);
}

/** Routes the browser proxy and screens depend on. */
export const DECLARED_ROUTES = [
  {
    method: "post",
    path: "/v2/auth/login",
    requestRef: "LoginRequest",
    requestRequired: true,
    responseStatus: "200",
    responseRef: "UserSession",
    parameters: [],
    problemResponses: ["400", "401", "429"],
    public: true,
  },
  {
    method: "get",
    path: "/v2/auth/me",
    responseStatus: "200",
    responseRef: "CurrentUser",
    parameters: [],
    problemResponses: ["401"],
  },
  {
    method: "post",
    path: "/v2/auth/logout",
    responseStatus: "204",
    parameters: [{ name: "X-CSRF-Token", in: "header", required: true }],
    problemResponses: ["401", "403"],
  },
  {
    method: "get",
    path: "/v2/classes",
    responseStatus: "200",
    responseInline: {
      required: ["items", "nextCursor"],
      properties: { items: { type: "array" }, nextCursor: { ref: "Cursor" } },
    },
    parameters: [],
    problemResponses: ["401"],
  },
  {
    method: "get",
    path: "/v2/classes/{classId}/labs",
    responseStatus: "200",
    responseInline: {
      required: ["items", "nextCursor"],
      properties: { items: { type: "array" }, nextCursor: { ref: "Cursor" } },
    },
    parameters: [
      { name: "limit", in: "query", required: false },
      { name: "cursor", in: "query", required: false },
    ],
    problemResponses: ["401", "404"],
  },
  {
    method: "post",
    path: "/v2/classes/{classId}/labs",
    requestRef: "LabCreateRequest",
    requestRequired: true,
    responseStatus: "201",
    responseRef: "Lab",
    parameters: [
      { name: "Idempotency-Key", in: "header", required: true },
      { name: "X-CSRF-Token", in: "header", required: true },
    ],
    problemResponses: ["400", "401", "403", "404", "409", "413"],
  },
  {
    method: "get",
    path: "/v2/labs/{labId}",
    responseStatus: "200",
    responseRef: "LabDetail",
    parameters: [],
    problemResponses: ["401", "404"],
  },
];

/** Schemas, required members and the exact bounds the screens validate. */
export const DECLARED_SCHEMAS = {
  User: {
    required: ["id", "displayName", "role"],
    fields: {
      id: { type: "string" },
      displayName: { type: "string" },
      role: { ref: "Role" },
    },
  },
  Role: { enum: ["admin", "ta", "student"] },
  LoginRequest: {
    required: ["email", "password"],
    fields: {
      email: { type: "string" },
      password: { type: "string" },
    },
  },
  UserSession: {
    required: ["user", "csrfToken", "expiresAt"],
    fields: {
      user: { ref: "User" },
      csrfToken: { type: "string" },
      expiresAt: { type: "string" },
    },
  },
  CurrentUser: {
    required: ["user", "classes"],
    fields: {
      user: { ref: "User" },
      classes: { type: "array", items: { ref: "ClassSummary" } },
    },
  },
  ClassSummary: {
    required: ["id", "name"],
    fields: {
      id: { type: "string" },
      name: { type: "string" },
    },
  },
  Constraint: {
    required: ["id", "text", "deduction"],
    fields: {
      id: { type: "string", pattern: "^[A-Za-z][A-Za-z0-9_-]{0,31}$" },
      text: { type: "string", minLength: 1, maxLength: 4096 },
      deduction: { type: "number", minimum: 0, maximum: 10, multipleOf: 0.01 },
    },
  },
  TestItem: {
    required: ["id", "input", "expectedOutput", "sample"],
    fields: {
      id: { type: "string", minLength: 1, maxLength: 64 },
      input: { type: "string" },
      expectedOutput: { type: "string" },
      sample: { type: "boolean" },
    },
  },
  Limits: {
    required: ["cpuSeconds", "memoryMiB"],
    fields: {
      cpuSeconds: { type: "integer", minimum: 1, maximum: 5 },
      memoryMiB: { type: "integer", minimum: 64, maximum: 512 },
    },
  },
  LabCreateRequest: {
    required: ["title", "statement", "opensAt", "deadline", "constraints", "tests", "limits"],
    fields: {
      title: { type: "string", minLength: 1, maxLength: 200 },
      statement: { type: "string", minLength: 1, maxLength: 65536 },
      opensAt: { type: "string", format: "date-time" },
      deadline: { type: "string", format: "date-time" },
      constraints: { type: "array", minItems: 1, maxItems: 20, items: { ref: "Constraint" } },
      tests: { type: "array", minItems: 1, maxItems: 500, items: { ref: "TestItem" } },
      limits: { ref: "Limits" },
    },
  },
  Lab: {
    required: [
      "id",
      "classId",
      "title",
      "statement",
      "opensAt",
      "deadline",
      "revision",
      "constraints",
      "limits",
      "modelNotice",
    ],
    fields: {
      id: { type: "string" },
      classId: { type: "string" },
      title: { type: "string" },
      statement: { type: "string" },
      opensAt: { type: "string", format: "date-time" },
      deadline: { type: "string", format: "date-time" },
      revision: { type: "integer", minimum: 1 },
      constraints: { type: "array", items: { ref: "Constraint" } },
      limits: { ref: "Limits" },
      modelNotice: { type: "string" },
    },
  },
  LabDetail: {
    inherits: "Lab",
    required: ["sampleTests"],
    fields: {
      sampleTests: { type: "array", items: { ref: "TestItem" } },
    },
  },
};

/** Bounds taken from the prose contract, mirrored in `apps/web/lib/lab-form.ts`. */
const DECLARED_BOUNDS = {
  Constraint: [{ field: "deduction", keyword: "maximum", value: 10 }],
  Limits: [
    { field: "cpuSeconds", keyword: "maximum", value: 5 },
    { field: "memoryMiB", keyword: "maximum", value: 512 },
  ],
  LabCreateRequest: [
    { field: "title", keyword: "maxLength", value: 200 },
    { field: "statement", keyword: "maxLength", value: 65536 },
    { field: "constraints", keyword: "maxItems", value: 20 },
    { field: "tests", keyword: "maxItems", value: 500 },
  ],
};

function isObject(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function tailRef(ref) {
  if (typeof ref !== "string" || !ref.startsWith("#/components/schemas/")) return null;
  return ref.split("/").at(-1) ?? null;
}

function resolvePointer(document, ref) {
  if (typeof ref !== "string" || !ref.startsWith("#/")) return undefined;
  return ref
    .slice(2)
    .split("/")
    .map((segment) => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
    .reduce((node, segment) => (isObject(node) ? node[segment] : undefined), document);
}

function flattenSchema(document, schema, seen = new Set()) {
  const result = { required: new Set(), properties: {} };
  if (!isObject(schema)) return result;

  if (typeof schema.$ref === "string") {
    if (seen.has(schema.$ref)) return result;
    seen.add(schema.$ref);
    const target = resolvePointer(document, schema.$ref);
    const resolved = flattenSchema(document, target, seen);
    for (const field of resolved.required) result.required.add(field);
    Object.assign(result.properties, resolved.properties);
  }
  if (Array.isArray(schema.allOf)) {
    for (const part of schema.allOf) {
      const resolved = flattenSchema(document, part, seen);
      for (const field of resolved.required) result.required.add(field);
      Object.assign(result.properties, resolved.properties);
    }
  }
  for (const field of Array.isArray(schema.required) ? schema.required : []) {
    result.required.add(field);
  }
  if (isObject(schema.properties)) {
    Object.assign(result.properties, schema.properties);
  }
  return result;
}

function compareSlot(actual, declared, label, failures, document) {
  if (declared.ref === undefined && isObject(actual) && typeof actual.$ref === "string") {
    const resolved = resolvePointer(document, actual.$ref);
    if (isObject(resolved)) {
      actual = resolved;
    } else {
      failures.push(`${label}: cannot resolve $ref ${actual.$ref}`);
      return;
    }
  }
  if (declared.ref !== undefined) {
    const actualRef = typeof actual.$ref === "string" ? tailRef(actual.$ref) : null;
    if (actualRef !== declared.ref) {
      failures.push(`${label}: expected $ref ${declared.ref}, found ${actualRef ?? "none"}`);
    }
    return;
  }
  const actualType = actual.type;
  const typeMatches = Array.isArray(actualType)
    ? actualType.includes(declared.type)
    : actualType === declared.type;
  if (!typeMatches) {
    failures.push(
      `${label}: expected type ${declared.type}, found ${JSON.stringify(actualType ?? null)}`,
    );
  }
  for (const key of [
    "format",
    "minLength",
    "maxLength",
    "minimum",
    "maximum",
    "multipleOf",
    "pattern",
    "minItems",
    "maxItems",
  ]) {
    if (declared[key] !== undefined && actual[key] !== declared[key]) {
      failures.push(
        `${label}.${key}: expected ${JSON.stringify(declared[key])}, found ${JSON.stringify(actual[key] ?? null)}`,
      );
    }
  }
  if (declared.items !== undefined) {
    if (!isObject(actual.items)) {
      failures.push(`${label}.items: expected an item schema`);
    } else {
      compareSlot(actual.items, declared.items, `${label}.items`, failures, document);
    }
  }
}

function operationParameters(document, pathItem, operation) {
  const merged = [];
  for (const parameter of Array.isArray(pathItem.parameters) ? pathItem.parameters : []) {
    merged.push(parameter);
  }
  for (const parameter of Array.isArray(operation.parameters) ? operation.parameters : []) {
    merged.push(parameter);
  }
  return merged
    .map((parameter) =>
      isObject(parameter) && typeof parameter.$ref === "string"
        ? resolvePointer(document, parameter.$ref)
        : parameter,
    )
    .filter(isObject);
}

function checkResponseSchema(document, route, schema, failures) {
  const label = `${route.method.toUpperCase()} ${route.path} ${route.responseStatus}`;
  if (route.responseRef !== undefined) {
    const actualRef = typeof schema?.$ref === "string" ? tailRef(schema.$ref) : null;
    if (actualRef !== route.responseRef) {
      failures.push(`${label}: expected $ref ${route.responseRef}, found ${actualRef ?? "none"}`);
    }
    return;
  }
  if (route.responseInline === undefined) return;
  const flattened = flattenSchema(document, schema);
  for (const field of route.responseInline.required) {
    if (!flattened.required.has(field)) {
      failures.push(`${label}: response no longer requires "${field}"`);
    }
    const slot = flattened.properties[field];
    if (!isObject(slot)) {
      failures.push(`${label}: response has no "${field}" property`);
      continue;
    }
    compareSlot(
      slot,
      route.responseInline.properties[field],
      `${label}.${field}`,
      failures,
      document,
    );
  }
}

export function checkDocument(document) {
  const failures = [];

  if (
    !isObject(document) ||
    typeof document.openapi !== "string" ||
    !document.openapi.startsWith("3.1")
  ) {
    return ["document: expected an OpenAPI 3.1 object"];
  }
  if (!isObject(document.paths)) {
    return ["document: expected paths"];
  }

  for (const route of DECLARED_ROUTES) {
    const label = `${route.method.toUpperCase()} ${route.path}`;
    const pathItem = document.paths[route.path];
    if (!isObject(pathItem)) {
      failures.push(`${label}: path is missing from the OpenAPI document`);
      continue;
    }
    const operation = pathItem[route.method];
    if (!isObject(operation)) {
      failures.push(`${label}: method is missing from the OpenAPI document`);
      continue;
    }

    if (route.public === true) {
      if (!Array.isArray(operation.security) || operation.security.length !== 0) {
        failures.push(`${label}: expected an empty security array (public route)`);
      }
    }
    if (route.requestRef !== undefined) {
      const body = operation.requestBody;
      if (!isObject(body) || (route.requestRequired === true && body.required !== true)) {
        failures.push(`${label}: request body is missing or no longer required`);
      } else {
        const schema = body.content?.["application/json"]?.schema;
        const actualRef = typeof schema?.$ref === "string" ? tailRef(schema.$ref) : null;
        if (actualRef !== route.requestRef) {
          failures.push(
            `${label}: expected JSON request $ref ${route.requestRef}, found ${actualRef ?? "none"}`,
          );
        }
      }
    }

    const parameters = operationParameters(document, pathItem, operation);
    for (const declared of route.parameters) {
      const actual = parameters.find(
        (parameter) => parameter.name === declared.name && parameter.in === declared.in,
      );
      if (actual === undefined) {
        failures.push(`${label}: missing ${declared.in} parameter ${declared.name}`);
      } else if ((actual.required === true) !== declared.required) {
        failures.push(
          `${label}: parameter ${declared.name} required is ${actual.required === true}, expected ${declared.required}`,
        );
      }
    }

    const response = operation.responses?.[route.responseStatus];
    if (!isObject(response)) {
      failures.push(`${label}: response ${route.responseStatus} is missing`);
    } else {
      checkResponseSchema(
        document,
        route,
        response.content?.["application/json"]?.schema,
        failures,
      );
    }

    for (const status of route.problemResponses ?? []) {
      const declaredResponse = operation.responses?.[status];
      const resolvedResponse =
        isObject(declaredResponse) && typeof declaredResponse.$ref === "string"
          ? resolvePointer(document, declaredResponse.$ref)
          : declaredResponse;
      if (
        !isObject(resolvedResponse) ||
        !isObject(resolvedResponse.content?.["application/problem+json"])
      ) {
        failures.push(`${label}: response ${status} no longer documents application/problem+json`);
      }
    }
  }

  for (const [schemaName, declared] of Object.entries(DECLARED_SCHEMAS)) {
    const schema = document.components?.schemas?.[schemaName];
    if (!isObject(schema)) {
      failures.push(`${schemaName}: schema is missing from the OpenAPI document`);
      continue;
    }

    if (declared.enum !== undefined) {
      const actualEnum = Array.isArray(schema.enum) ? schema.enum : [];
      if (JSON.stringify(actualEnum) !== JSON.stringify(declared.enum)) {
        failures.push(
          `${schemaName}.enum: expected ${JSON.stringify(declared.enum)}, found ${JSON.stringify(actualEnum)}`,
        );
      }
      continue;
    }

    const flattened = flattenSchema(document, schema);
    for (const field of declared.required) {
      if (!flattened.required.has(field)) {
        failures.push(`${schemaName}: now optional or missing required field "${field}"`);
      }
      const slot = flattened.properties[field];
      if (!isObject(slot)) {
        failures.push(`${schemaName}.${field}: property is missing from the OpenAPI document`);
      } else {
        compareSlot(slot, declared.fields[field], `${schemaName}.${field}`, failures, document);
      }
    }
    const inheritedRequired =
      declared.inherits === undefined ? [] : (DECLARED_SCHEMAS[declared.inherits]?.required ?? []);
    for (const field of inheritedRequired) {
      if (!flattened.required.has(field)) {
        failures.push(`${schemaName}: no longer requires inherited field "${field}"`);
      }
    }
    for (const field of flattened.required) {
      if (!declared.required.includes(field) && !inheritedRequired.includes(field)) {
        failures.push(
          `${schemaName}: OpenAPI now requires "${field}"; update apps/web/lib/api-types.ts`,
        );
      }
    }

    const bounds = DECLARED_BOUNDS[schemaName];
    if (bounds !== undefined) {
      for (const bound of bounds) {
        const slot = flattened.properties[bound.field];
        if (isObject(slot) && slot[bound.keyword] !== bound.value) {
          failures.push(
            `${schemaName}.${bound.field}.${bound.keyword}: contract bound is ${JSON.stringify(slot[bound.keyword] ?? null)} (expected ${bound.value})`,
          );
        }
      }
    }
  }

  return failures;
}

async function main(argv) {
  let openapiPath = DEFAULT_OPENAPI_PATH;
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--openapi") {
      const value = argv[index + 1];
      if (value === undefined) {
        writeStderr("openapi-drift-check: --openapi requires a file path");
        process.exit(2);
      }
      openapiPath = value;
      index += 1;
    } else {
      writeStderr(`openapi-drift-check: unknown argument ${argument}`);
      process.exit(2);
    }
  }

  let document;
  try {
    document = JSON.parse(await readFile(openapiPath, "utf8"));
  } catch (error) {
    writeStderr(`openapi-drift-check: cannot read ${openapiPath}: ${error.message}`);
    process.exit(2);
  }

  const failures = checkDocument(document);
  if (failures.length > 0) {
    writeStderr(`OpenAPI drift detected (${failures.length}):`);
    for (const failure of failures) {
      writeStderr(`  - ${failure}`);
    }
    process.exitCode = 1;
    return;
  }

  writeStdout(
    `OpenAPI drift check passed (${DECLARED_ROUTES.length} routes, ${Object.keys(DECLARED_SCHEMAS).length} schemas).`,
  );
}

const entryPoint = process.argv[1];
if (entryPoint !== undefined && import.meta.url === pathToFileURL(path.resolve(entryPoint)).href) {
  await main(process.argv.slice(2));
}
