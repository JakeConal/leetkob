import { spawn } from "node:child_process";
import process from "node:process";

const npmCli = process.env.npm_execpath;
if (npmCli === undefined) throw new Error("npm must launch the development workspace");

const children = ["dev:api", "dev:web"].map((script) =>
  spawn(process.execPath, [npmCli, "run", script], { stdio: "inherit" }),
);

let stopping = false;
function stop(exitCode = 0) {
  if (stopping) return;
  stopping = true;
  for (const child of children) {
    if (!child.killed) child.kill();
  }
  process.exitCode = exitCode;
}

for (const child of children) {
  child.once("error", () => stop(1));
  child.once("exit", (code) => stop(code ?? 1));
}

process.once("SIGINT", () => stop());
process.once("SIGTERM", () => stop());
