#!/usr/bin/env node
/* global process */

// Runs the Go-side portion of a root quality command inside apps/backend.
// A missing or wrong Go toolchain fails the check: coverage that silently
// skips the Go workspace is not coverage.
//
// Usage: node scripts/go-check.mjs <format|vet|build|test|security>

import { spawnSync } from "node:child_process";
import { constants as fsConstants } from "node:fs";
import { accessSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";

const supported = ["format", "vet", "build", "test", "security"];
const subcommand = process.argv[2];
if (!supported.includes(subcommand)) {
  process.stderr.write(`usage: node scripts/go-check.mjs <${supported.join("|")}>\n`);
  process.exitCode = 2;
} else {
  process.exitCode = run(subcommand);
}

function run(subcommand) {
  const backendDir = join(dirname(process.argv[1]), "..", "apps", "backend");
  if (!existsSync(join(backendDir, "go.mod"))) {
    process.stderr.write(`go-check: missing ${join(backendDir, "go.mod")}\n`);
    return 1;
  }

  const requiredGoVersion = "go1.27.4";
  const go = resolveGo();
  if (!go) {
    process.stderr.write(`go-check: Go toolchain not found; install ${requiredGoVersion}\n`);
    return 1;
  }
  if (go.reported !== requiredGoVersion) {
    process.stderr.write(`go-check: expected ${requiredGoVersion}, found ${go.reported}\n`);
    process.stderr.write("go-check: pin the toolchain before trusting these results\n");
    return 1;
  }

  const commands = {
    // Formatting: any gofmt delta fails the root Format result. gofmt is
    // invoked directly because `go fmt` rewrites files instead of
    // reporting them.
    format: {
      args: ["-l", "."],
      check: (out) => out.trim().length === 0,
      describe: "gofmt -l",
      binary: "gofmt",
    },
    // Vet participates in the root Lint result.
    vet: { args: ["vet", "./..."], check: () => true, describe: "go vet ./..." },
    // Compilation is Go's static type check: it participates in Typecheck.
    build: { args: ["build", "./..."], check: () => true, describe: "go build ./..." },
    // Tests run with the race detector; they participate in the root Test result.
    test: { args: ["test", "-race", "./..."], check: () => true, describe: "go test -race ./..." },
    // Pinned vulnerability scanning participates in the security job.
    security: {
      args: ["run", "golang.org/x/vuln/cmd/govulncheck@v1.10.0", "./..."],
      check: () => true,
      describe: "govulncheck v1.10.0",
    },
  };
  const command = commands[subcommand];
  process.stdout.write(`go-check: ${command.describe} (apps/backend, ${go.reported})\n`);

  const executable = command.binary === "gofmt" ? resolveGofmt(go.command) : go.command;
  const needsStdout = typeof command.check === "function";
  const result = spawnSync(executable, command.args, {
    cwd: backendDir,
    encoding: needsStdout ? "utf8" : null,
    stdio: ["ignore", needsStdout ? "pipe" : "inherit", "inherit"],
  });
  if (result.error) {
    process.stderr.write(`go-check: failed to run ${command.describe}: ${result.error.message}\n`);
    return 1;
  }
  if (typeof result.stdout === "string" && !command.check(result.stdout)) {
    for (const file of result.stdout.trim().split("\n")) {
      if (file !== "") process.stderr.write(`gofmt: ${file}\n`);
    }
    process.stderr.write("go-check: gofmt reported unformatted files\n");
    return 1;
  }
  if (result.status !== 0) {
    if (typeof result.stdout === "string" && result.stdout.trim() !== "") {
      process.stderr.write(result.stdout);
      if (!result.stdout.endsWith("\n")) process.stderr.write("\n");
    }
    process.stderr.write(`go-check: ${command.describe} failed\n`);
    return result.status ?? 1;
  }
  return 0;
}

function resolveGofmt(goCommand) {
  return join(dirname(goCommand), "gofmt");
}

function resolveGo() {
  // Scan candidate locations directly so the executed path is fixed and
  // absolute; a bare PATH lookup would be a security review hotspot.
  const candidates = [
    process.env.GO,
    "/snap/go/current/bin/go",
    "/usr/local/go/bin/go",
    join(dirname(process.execPath), "go"),
    ...(process.env.PATH ?? "")
      .split(":")
      .filter(Boolean)
      .map((entry) => join(entry, "go")),
  ];
  for (const candidate of candidates) {
    if (candidate === undefined) continue;
    try {
      if (existsSync(candidate) && accessSync(candidate, fsConstants.X_OK) === undefined) {
        const probe = spawnSync(candidate, ["version"], { encoding: "utf8" });
        if (probe.status === 0) {
          return { command: candidate, reported: probe.stdout.trim().split(/\s+/)[2] ?? "" };
        }
      }
    } catch {
      // Not executable at this candidate; try the next.
    }
  }
  return null;
}
