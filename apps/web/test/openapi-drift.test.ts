import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

const repoRoot = path.resolve(import.meta.dirname, "../../..");
const scriptPath = path.join(repoRoot, "scripts/openapi-drift-check.mjs");
const openapiPath = path.join(repoRoot, "docs/contract/pa3-openapi.json");

function runCheck(openapiFile: string): { status: number | null; stdout: string; stderr: string } {
  const result = spawnSync(process.execPath, [scriptPath, "--openapi", openapiFile], {
    cwd: repoRoot,
    encoding: "utf8",
  });
  return { status: result.status, stdout: result.stdout, stderr: result.stderr };
}

test("the checked-in contract matches the declared web routes and fields", () => {
  const result = runCheck(openapiPath);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /OpenAPI drift check passed \(7 routes, 12 schemas\)/);
});

test("drift in the contract fails the check with actionable messages", (t) => {
  const directory = mkdtempSync(path.join(tmpdir(), "leetkob-openapi-drift-"));
  t.after(() => {
    rmSync(directory, { recursive: true, force: true });
  });

  const document = JSON.parse(readFileSync(openapiPath, "utf8")) as {
    paths: Record<string, unknown>;
    components: {
      schemas: Record<
        string,
        { required?: string[]; properties: Record<string, Record<string, unknown>> }
      >;
    };
  };

  const labSchema = document.components.schemas["Lab"];
  const constraintSchema = document.components.schemas["Constraint"];
  const modelNotice = labSchema?.properties["modelNotice"];
  const deduction = constraintSchema?.properties["deduction"];
  assert.ok(labSchema !== undefined);
  assert.ok(modelNotice !== undefined);
  assert.ok(deduction !== undefined);

  delete document.paths["/v2/labs/{labId}"];
  modelNotice["type"] = "number";
  labSchema.required = (labSchema.required ?? []).filter((field) => field !== "modelNotice");
  deduction["maximum"] = 11;

  const labsPath = document.paths["/v2/classes/{classId}/labs"] as {
    post: { parameters: Array<{ $ref?: string; name?: string }> };
  };
  labsPath.post.parameters = labsPath.post.parameters.filter(
    (parameter) => parameter.$ref !== "#/components/parameters/CsrfToken",
  );

  const fixturePath = path.join(directory, "pa3-openapi.drifted.json");
  writeFileSync(fixturePath, JSON.stringify(document, null, 2));

  const result = runCheck(fixturePath);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /OpenAPI drift detected/);
  assert.match(result.stderr, /GET \/v2\/labs\/\{labId\}: path is missing/);
  assert.match(result.stderr, /Lab\.modelNotice: expected type string, found "number"/);
  assert.match(result.stderr, /Lab: now optional or missing required field "modelNotice"/);
  assert.match(result.stderr, /Constraint\.deduction\.maximum: contract bound is 11/);
  assert.match(
    result.stderr,
    /POST \/v2\/classes\/\{classId\}\/labs: missing header parameter X-CSRF-Token/,
  );
});
