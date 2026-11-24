#!/usr/bin/env node
/* global URL, process */

// Deterministic contract drift check: every route the live web client calls
// must exist in the published PA#3 OpenAPI document, with the same methods.
// Run after changing the contract or the web path table.

import { readFileSync } from "node:fs";
import { join } from "node:path";

const root = new URL("../", import.meta.url).pathname;
const openapi = JSON.parse(readFileSync(join(root, "docs/contract/pa3-openapi.json"), "utf8"));

const pathsSource = readFileSync(join(root, "apps/web/lib/live/paths.ts"), "utf8");
const templateBlock = pathsSource.slice(
  pathsSource.indexOf("liveApiPathTemplates"),
  pathsSource.indexOf("} as const;", pathsSource.indexOf("liveApiPathTemplates")),
);
const templates = [...templateBlock.matchAll(/"(\/v2\/[^"]+)"/g)].map((match) => match[1]);

if (templates.length === 0) {
  console.error("drift: no /v2 path templates found in apps/web/lib/live/paths.ts");
  process.exit(1);
}

const documented = Object.keys(openapi.paths ?? {});
const failures = [];
for (const template of templates) {
  if (!documented.includes(template)) {
    failures.push(`web calls ${template} but the OpenAPI document does not publish it`);
  }
}
for (const path of documented) {
  if (!templates.includes(path)) {
    failures.push(`OpenAPI publishes ${path} but the web client has no path template`);
  }
}

if (failures.length > 0) {
  console.error(`drift: ${failures.length} mismatch(es)`);
  for (const failure of failures) console.error(`  - ${failure}`);
  process.exit(1);
}
process.stdout.write(`contract drift check passed (${templates.length} routes)\n`);
