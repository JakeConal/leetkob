/* global process, console */
import { createHash } from "node:crypto";
import { lstatSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, URL } from "node:url";

export const officialSource = JSON.parse(
  readFileSync(new URL("../../evaluation/official-source.json", import.meta.url), "utf8"),
);

function filesAt(root, prefix = "") {
  return readdirSync(join(root, prefix))
    .sort((a, b) => a.localeCompare(b))
    .flatMap((name) => {
      if (!prefix && name === ".git") return [];
      const path = prefix ? `${prefix}/${name}` : name;
      const stat = lstatSync(join(root, path));
      if (stat.isSymbolicLink()) throw new Error(`Symlink rejected: ${path}`);
      if (stat.isDirectory()) return filesAt(root, path);
      if (!stat.isFile()) throw new Error(`Non-file rejected: ${path}`);
      return [path];
    });
}

export function verifyOfficial(root) {
  const actual = filesAt(root);
  const expected = Object.keys(officialSource.sha256).sort((a, b) => a.localeCompare(b));
  if (JSON.stringify(actual.sort((a, b) => a.localeCompare(b))) !== JSON.stringify(expected)) {
    throw new Error("Official source file inventory differs from the pinned manifest");
  }
  for (const path of expected) {
    const hash = createHash("sha256")
      .update(readFileSync(join(root, path)))
      .digest("hex");
    if (hash !== officialSource.sha256[path]) throw new Error(`SHA-256 mismatch: ${path}`);
  }
  return expected.length;
}

// Trusted preparation only. Run the predictor later with ONLY this output mounted.
// Never copy the dataset recursively: labels, scorer, tests and metadata stay trusted.
export function stagePredictionInputs(root, destination) {
  const source = resolve(root);
  const output = resolve(destination);
  if (output === source || output.startsWith(`${source}/`)) {
    throw new Error("Prediction destination must be outside the source dataset");
  }
  const paths = filesAt(source).filter((path) =>
    /^(problems\/[a-z0-9-]+\/(statement\.md|constraints\.json)|solutions\/[a-z0-9-]+\/s\d+\.cpp)$/.test(
      path,
    ),
  );
  if (!paths.length) throw new Error("Dataset contains no prediction inputs");
  // Exclusive creation prevents stale labels or other files in a reused directory.
  mkdirSync(output);
  for (const path of paths) {
    const target = join(output, path);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, readFileSync(join(source, path)), { flag: "wx" });
  }
  return paths;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [operation, source, destination, ...extra] = process.argv.slice(2);
  if (
    extra.length ||
    !source ||
    !["verify", "stage"].includes(operation) ||
    (operation === "stage" ? !destination : destination)
  ) {
    throw new Error(
      "Usage: node scripts/evaluation/sources.mjs verify <official-dir> | stage <dataset-dir> <new-input-dir>",
    );
  }
  if (operation === "verify") console.log(`Verified ${verifyOfficial(source)} pinned files`);
  else
    console.log(
      `Staged ${stagePredictionInputs(source, destination).length} prediction input files`,
    );
}
