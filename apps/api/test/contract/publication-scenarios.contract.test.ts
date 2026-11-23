import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  InMemoryIdempotencyRepository,
  InMemoryLabRepository,
} from "../../src/adapters/in-memory.js";
import { createApiApplication } from "../../src/app-factory.js";
import { issueToken } from "../../src/auth/token.js";
import type { Clock, LabRepository } from "../../src/domain/ports.js";
import type { Lab, Role } from "../../src/domain/types.js";

interface ScenarioIdentity {
  readonly subject: string;
  readonly role: Role;
  readonly classIds: string[];
}

interface ScenarioLab {
  readonly id: string;
  readonly classId: string;
  readonly status: "ready";
  readonly pendingReviews: number;
  readonly submissionDeadline: string;
}

interface ExpectedBody {
  readonly status: number;
  readonly contentType?: string;
  readonly body?: Record<string, unknown>;
}

interface RequestStep {
  readonly type: "request";
  readonly identity?: string;
  readonly identityRef?: string;
  readonly tokenKind?: "absent" | "malformed" | "tampered" | "expired";
  readonly method?: string;
  readonly path: string;
  readonly idempotencyKey?: string;
  readonly body?: string;
  readonly expected?: ExpectedBody;
  readonly expectedBodyEqualTo?: string;
  readonly saveBodyAs?: string;
}

interface Scenario {
  readonly name: string;
  readonly failLabSave?: boolean;
  readonly labs: ScenarioLab[];
  readonly identities?: Record<string, ScenarioIdentity>;
  readonly steps: readonly Record<string, unknown>[];
}

interface ScenarioFile {
  readonly secret: string;
  readonly nowSeconds: number;
  readonly instant: string;
  readonly identities: Record<string, ScenarioIdentity>;
  readonly labs: Record<string, string>;
  readonly deadlines: { readonly passed: string; readonly future: string };
  readonly scenarios: Scenario[];
}

const scenariosUrl = new URL(
  "../../../../docs/contract/fixtures/publication-scenarios.json",
  import.meta.url,
);
const file: ScenarioFile = JSON.parse(readFileSync(scenariosUrl, "utf8")) as ScenarioFile;

const substitutions: Record<string, string> = {
  "{instant}": file.instant,
  "{passed}": file.deadlines.passed,
  "{future}": file.deadlines.future,
};
for (const [name, id] of Object.entries(file.labs)) {
  substitutions[`{${name}}`] = id;
}

function resolve(value: string): string {
  let resolved = value;
  for (const [placeholder, replacement] of Object.entries(substitutions)) {
    resolved = resolved.split(placeholder).join(replacement);
  }
  return resolved;
}

function resolveDeep(value: unknown): unknown {
  if (typeof value === "string") return resolve(value);
  if (Array.isArray(value)) return value.map((item) => resolveDeep(item));
  if (value !== null && typeof value === "object") {
    const resolved: Record<string, unknown> = {};
    for (const [key, member] of Object.entries(value)) {
      resolved[key] = resolveDeep(member);
    }
    return resolved;
  }
  return value;
}

class FixedClock implements Clock {
  readonly #instant: Date;

  constructor(instant: string) {
    this.#instant = new Date(instant);
  }

  now(): Date {
    return new Date(this.#instant);
  }
}

function scenarioLab(lab: ScenarioLab): Lab {
  return {
    id: resolve(lab.id),
    classId: lab.classId,
    status: "ready",
    pendingReviews: lab.pendingReviews,
    submissionDeadline: resolve(lab.submissionDeadline),
  };
}

async function start(scenario: Scenario): Promise<{
  readonly baseUrl: string;
  readonly labs: InMemoryLabRepository;
  readonly tokenFor: (identity: ScenarioIdentity, expiresAt?: number) => string;
  readonly close: () => Promise<void>;
}> {
  const labs = new InMemoryLabRepository(scenario.labs.map(scenarioLab));
  const repository: LabRepository = scenario.failLabSave
    ? {
        findById: (id) => labs.findById(id),
        save: async () => {
          throw new Error("database-password and internal table must not leak");
        },
      }
    : labs;
  const application = await createApiApplication({
    authSecret: file.secret,
    clock: new FixedClock(file.instant),
    idempotency: new InMemoryIdempotencyRepository(),
    labs: repository,
    nowSeconds: () => file.nowSeconds,
  });
  await application.listen(0, "127.0.0.1");

  const tokenFor = (identity: ScenarioIdentity, expiresAt?: number): string =>
    issueToken(
      { subject: identity.subject, role: identity.role, classIds: identity.classIds },
      file.secret,
      expiresAt ?? file.nowSeconds + 60,
    );

  return {
    baseUrl: await application.getUrl(),
    labs,
    tokenFor,
    close: () => application.close(),
  };
}

function identityFor(scenario: Scenario, step: RequestStep): ScenarioIdentity {
  const name = step.identity ?? step.identityRef;
  if (name !== undefined) {
    const local = scenario.identities?.[name];
    if (local !== undefined) return local;
    const global = file.identities[name];
    if (global !== undefined) return global;
  }
  return file.identities["ta"]!;
}

async function performRequest(
  baseUrl: string,
  step: RequestStep,
  tokenFor: (identity: ScenarioIdentity, expiresAt?: number) => string,
): Promise<Response> {
  const headers = new Headers();
  const identity = identityFor(scenarioCurrent, step);
  switch (step.tokenKind) {
    case "absent":
      break;
    case "malformed":
      headers.set("authorization", "Bearer not-a-token");
      break;
    case "tampered": {
      const valid = tokenFor(identity);
      headers.set("authorization", `Bearer ${valid.slice(0, -1)}x`);
      break;
    }
    case "expired":
      headers.set("authorization", `Bearer ${tokenFor(identity, file.nowSeconds)}`);
      break;
    default:
      headers.set("authorization", `Bearer ${tokenFor(identity)}`);
  }
  if (step.idempotencyKey !== undefined) {
    headers.set("idempotency-key", step.idempotencyKey);
  }

  return fetch(`${baseUrl}${resolve(step.path)}`, {
    method: step.method ?? "POST",
    headers,
    ...(step.body === undefined ? {} : { body: step.body }),
  });
}

let scenarioCurrent: Scenario = file.scenarios[0]!;

for (const scenario of file.scenarios) {
  test(`publication scenario: ${scenario.name}`, async (context) => {
    scenarioCurrent = scenario;
    const application = await start(scenario);
    context.after(application.close);

    const savedBodies = new Map<string, Record<string, unknown>>();

    for (const [index, rawStep] of scenario.steps.entries()) {
      const step = rawStep as Record<string, unknown>;
      const kind = step["type"];

      if (kind === "request") {
        const response = await performRequest(
          application.baseUrl,
          rawStep as unknown as RequestStep,
          application.tokenFor,
        );
        await assertExpected(rawStep as unknown as RequestStep, response, savedBodies);
        continue;
      }

      if (kind === "saveLab") {
        const lab = scenarioLab(step["lab"] as ScenarioLab);
        await application.labs.save(lab);
        continue;
      }

      if (kind === "expectLab") {
        const labId = resolve(String(step["labId"]));
        const stored = await application.labs.findById(labId);
        assert.equal(stored?.status, step["status"]);
        continue;
      }

      if (kind === "concurrent") {
        const requests = (step["requests"] ?? []) as RequestStep[];
        const responses = await Promise.all(
          requests.map((request) =>
            performRequest(application.baseUrl, request, application.tokenFor),
          ),
        );
        const statuses = responses.map((response) => response.status).sort((a, b) => a - b);
        assert.deepEqual(statuses, step["expectedStatuses"]);
        continue;
      }

      if (kind === "assertNotContains") {
        const text = JSON.stringify(savedBodies.get(String(step["from"])));
        for (const forbidden of (step["forbidden"] ?? []) as string[]) {
          assert.equal(text?.includes(forbidden), false, `body must not contain ${forbidden}`);
        }
        continue;
      }

      if (kind === "assertNotMatches") {
        const text = JSON.stringify(savedBodies.get(String(step["from"])));
        const pattern = new RegExp(String(step["pattern"]), "i");
        assert.doesNotMatch(text ?? "", pattern);
        continue;
      }

      assert.fail(`unknown step type at index ${index}: ${String(kind)}`);
    }
  });
}

async function assertExpected(
  step: RequestStep,
  response: Response,
  savedBodies: Map<string, Record<string, unknown>>,
): Promise<void> {
  let body: Record<string, unknown> | undefined;
  let read = false;
  const readBody = async (): Promise<Record<string, unknown>> => {
    if (!read) {
      body = (await response.json()) as Record<string, unknown>;
      read = true;
    }
    return body!;
  };

  if (step.expected !== undefined) {
    assert.equal(response.status, step.expected.status);
    const wantContentType = step.expected.contentType ?? "application/problem+json";
    assert.equal(
      (response.headers.get("content-type") ?? "").startsWith(wantContentType),
      true,
      `content-type must start with ${wantContentType}`,
    );
    assert.equal(response.headers.get("cache-control"), "no-store");
    if (step.expected.body !== undefined) {
      assert.deepEqual(await readBody(), resolveDeep(step.expected.body));
    }
  }
  if (step.expectedBodyEqualTo !== undefined) {
    assert.deepEqual(await readBody(), savedBodies.get(step.expectedBodyEqualTo));
  }
  if (step.saveBodyAs !== undefined) {
    savedBodies.set(step.saveBodyAs, await readBody());
  }
}
