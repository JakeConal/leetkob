/* global fetch, console, process, AbortSignal, setTimeout */
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { mkdir, readFile, writeFile, rename } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, URL } from "node:url";
import { promisify } from "node:util";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const stateLabel = ".local-dev/v2";
const stateDir = join(root, stateLabel);
const statePath = join(stateDir, "state.json");
const dataset = JSON.parse(await readFile(join(root, "scripts/e2e/data.json"), "utf8"));
const classA = "6f0c1a10-7b21-4a11-8c11-aaaaaaaaaaaa";
const classB = "6f0c1a10-7b21-4a11-8c11-bbbbbbbbbbbb";
const alphabetical = (left, right) => left.localeCompare(right);
const delay = (ms) => new Promise((done) => setTimeout(done, ms));
const sessions = new Map();
let state;

function port(value, fallback) {
  const selected = Number(value ?? fallback);
  assert(Number.isInteger(selected) && selected > 1024 && selected <= 65535, "Invalid E2E port");
  return selected;
}

async function save() {
  const temporary = `${statePath}.${process.pid}.tmp`;
  await writeFile(temporary, JSON.stringify(state, null, 2) + "\n", { mode: 0o600 });
  await rename(temporary, statePath);
}

async function load(create = false) {
  await mkdir(stateDir, { recursive: true, mode: 0o700 });
  try {
    state = JSON.parse(await readFile(statePath, "utf8"));
  } catch (error) {
    if (error.code !== "ENOENT" || !create) throw error;
    state = {
      version: 1,
      datasetVersion: dataset.version,
      createdAt: new Date().toISOString(),
      webPort: port(process.env.LOCAL_WEB_PORT, 3000),
      apiPort: port(process.env.LOCAL_API_PORT, 8080),
      postgresPort: port(process.env.LOCAL_POSTGRES_PORT, 5545),
      accounts: {},
      labs: {},
      submissions: {},
    };
    assert(
      new Set([state.webPort, state.apiPort, state.postgresPort]).size === 3,
      "Ports must differ",
    );
    await save();
  }
  assert.equal(state.version, 1, "Unsupported E2E state version");
  state.webPort = port(process.env.LOCAL_WEB_PORT, 3000);
  state.apiPort = port(process.env.LOCAL_API_PORT, 8080);
  state.postgresPort = port(process.env.LOCAL_POSTGRES_PORT, 5545);
  await save();
  for (const key of ["webPort", "apiPort", "postgresPort"]) port(state[key]);
}

function origin() {
  return `http://127.0.0.1:${state.webPort}`;
}

async function request(
  path,
  { method = "GET", body, session, key, expected = 200, retryRateLimit = 0 } = {},
) {
  const headers = { Origin: origin() };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (session) {
    headers.Cookie = session.cookie;
    headers["X-CSRF-Token"] = session.csrf;
  }
  if (key) headers["Idempotency-Key"] = key;
  const response = await fetch(origin() + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(30_000),
    redirect: "manual",
  });
  const data = response.status === 204 ? {} : await response.json();
  const retryAfter = Number(response.headers.get("retry-after"));
  if (
    response.status === 429 &&
    retryRateLimit > 0 &&
    Number.isInteger(retryAfter) &&
    retryAfter > 0 &&
    retryAfter <= 60
  ) {
    console.log(`Login throttled; honoring Retry-After (${retryAfter}s).`);
    await delay(Math.min(retryAfter + 1, 60) * 1000);
    return request(path, {
      method,
      body,
      session,
      key,
      expected,
      retryRateLimit: retryRateLimit - 1,
    });
  }
  assert.equal(
    response.status,
    expected,
    `${method} ${path}: HTTP ${response.status} (${data.title ?? "unexpected response"})`,
  );
  assert.equal(
    response.headers.get("cache-control"),
    "no-store",
    `${path}: protected response must not be cached`,
  );
  return { data, response };
}

async function login(name) {
  if (sessions.has(name)) return sessions.get(name);
  const account = state.accounts[name];
  assert(
    account?.password,
    `Missing synthetic password for ${name}; preserve .local-dev/v2/state.json with its database volume`,
  );
  const { data, response } = await request("/v2/auth/login", {
    method: "POST",
    body: { email: account.email, password: account.password },
    retryRateLimit: 2,
  });
  const cookie = response.headers
    .getSetCookie()
    .map((item) => item.split(";", 1)[0])
    .join("; ");
  assert(cookie && data.csrfToken, "Login must return a session cookie and CSRF proof");
  const session = { cookie, csrf: data.csrfToken };
  sessions.set(name, session);
  return session;
}

async function seed() {
  state.datasetVersion ??= dataset.version;
  if (state.datasetVersion !== dataset.version) {
    assert(
      Number.isInteger(dataset.version) && dataset.version > (state.datasetVersion ?? 0),
      "Dataset versions must increase",
    );
    state.datasetVersion = dataset.version;
    state.createdAt = new Date().toISOString();
    state.labs = {};
    state.submissions = {};
    state.submissionRun = 0;
    await save();
  }
  if (process.argv.includes("--resubmit")) {
    state.submissionRun = (state.submissionRun ?? 0) + 1;
    state.submissions = {};
    await save();
  }
  const accounts = {
    admin: ["admin@assessment.leetkob.test", "Admin", "admin"],
    ta1: ["ta-a@assessment.leetkob.test", "TeachingAssistantA", "ta"],
    ta2: ["ta-b@assessment.leetkob.test", "TeachingAssistantB", "ta"],
    student1: ["student-a1@assessment.leetkob.test", "StudentA1", "student"],
    student2: ["student-a2@assessment.leetkob.test", "StudentA2", "student"],
  };
  const database = new URL("postgres://127.0.0.1/leetkob_local?sslmode=disable");
  database.username = "leetkob_local";
  database.password = (await readFile(join(stateDir, "database-password"), "utf8")).trim();
  assert(
    /^[A-Za-z0-9_-]{32,}$/.test(database.password),
    "Invalid generated local database password",
  );
  database.port = String(state.postgresPort);
  const passwords = {};
  for (const [name, [email, field, role]] of Object.entries(accounts)) {
    state.accounts[name] ??= { email, role, password: randomBytes(24).toString("base64url") };
    passwords[field] = state.accounts[name].password;
  }
  await save();
  await promisify(execFile)(join(stateDir, "bin/provision"), [], {
    cwd: root,
    env: {
      PATH: process.env.PATH,
      DATABASE_URL: database.href,
      ASSESSMENT_PASSWORDS: JSON.stringify(passwords),
    },
  });
  const ta = await login("ta1");
  const taOther = await login("ta2");
  const anchor = Date.parse(state.createdAt);
  assert(
    Date.now() < anchor + 30 * 86_400_000,
    "This dataset has expired; use a new E2E dataset after preserving the old volume",
  );
  const definitions = [
    ...dataset.scenarios.map((scenario) => ({ ...scenario, classID: classA, session: ta })),
    { key: "empty", title: "E2E: empty review queue", classID: classA, session: ta },
    {
      key: "closed",
      title: "E2E: submission deadline passed",
      classID: classA,
      session: ta,
      closed: true,
    },
    {
      key: "other-class",
      title: "E2E: another TA's private lab",
      classID: classB,
      session: taOther,
    },
  ];
  for (const item of definitions) {
    const body = {
      title: item.title,
      statement: dataset.statement,
      opensAt: new Date(anchor - 2 * 86_400_000).toISOString(),
      deadline: new Date(anchor + (item.closed ? -1 : 30) * 86_400_000).toISOString(),
      constraints: dataset.constraints,
      tests: dataset.tests,
      limits: { cpuSeconds: 2, memoryMiB: 128 },
    };
    const { data } = await request(`/v2/classes/${item.classID}/labs`, {
      method: "POST",
      body,
      session: item.session,
      key: `e2e-v${dataset.version}-${item.key}`,
      expected: 201,
    });
    if (state.labs[item.key])
      assert.equal(data.id, state.labs[item.key].id, "Seeding must reuse the original lab");
    state.labs[item.key] = {
      id: data.id,
      title: item.title,
      url: origin() + `/live/labs/${data.id}`,
    };
    await save();
  }
  const runSuffix = state.submissionRun ? `-run-${state.submissionRun}` : "";
  for (const item of dataset.scenarios) {
    const source = await readFile(join(root, "scripts/e2e/fixtures", item.source), "utf8");
    const { data } = await request(`/v2/labs/${state.labs[item.key].id}/submissions`, {
      method: "POST",
      body: { source },
      session: await login(item.student),
      key: `e2e-v${dataset.version}-${item.key}${runSuffix}`,
      expected: 202,
    });
    if (state.submissions[item.key])
      assert.equal(
        data.id,
        state.submissions[item.key].id,
        "Seeding must reuse the original submission",
      );
    state.submissions[item.key] = {
      id: data.id,
      sourceSha256: createHash("sha256").update(source).digest("hex"),
    };
    await save();
  }
  console.log("Seeded 5 accounts, 2 classes, 8 labs and 5 real submissions (idempotent).");
}

async function waitFor(description, check) {
  const deadline = Date.now() + 180_000;
  while (Date.now() < deadline) {
    const value = await check();
    if (value) return value;
    await delay(1000);
  }
  throw new Error(`Timed out waiting for ${description}; inspect .local-dev/v2/logs`);
}

async function smoke() {
  const report = { startedAt: new Date().toISOString(), modelOff: true, checks: [] };
  await writeFile(
    join(stateDir, "report.json"),
    JSON.stringify({ ...report, status: "running" }, null, 2) + "\n",
  );
  const ta = await login("ta1");
  const otherTA = await login("ta2");
  const student = await login("student1");
  for (const item of dataset.scenarios) {
    const lab = state.labs[item.key];
    const submission = state.submissions[item.key];
    assert(lab && submission, "Run npm run seed:local first");
    const owner = await login(item.student);
    const judged = await waitFor(item.key + " judging", async () => {
      const { data } = await request(`/v2/submissions/${submission.id}`, { session: owner });
      return data.state === "judged" && data;
    });
    assert.equal(judged.judge.verdict, item.judge, `${item.key}: incorrect judge verdict`);
    assert.deepEqual(
      Object.keys(judged).sort(alphabetical),
      ["id", "judge", "labId", "receivedAt", "state"],
      "Student result leaked private fields",
    );
    for (const test of judged.judge.tests) {
      assert.deepEqual(
        Object.keys(test).sort(alphabetical),
        ["id", "timeMs", "verdict"],
        "Per-test result leaked hidden test data",
      );
    }
    const queue = await waitFor(item.key + " checking", async () => {
      const { data } = await request(`/v2/labs/${lab.id}/reviews`, { session: ta });
      return !data.processingPending && data;
    });
    if (item.judge === "AC") {
      assert.equal(queue.items.length, 2, "Accepted submission must produce both findings");
      const hard = queue.items.find((finding) => finding.constraintId === "C1");
      const semantic = queue.items.find((finding) => finding.constraintId === "C2");
      assert.equal(
        hard.checkerVerdict,
        item.hardRule,
        `${item.key}: real hard-rule analysis failed`,
      );
      if (item.hardRule === "violation")
        assert(hard.evidence.length > 0, "Violation requires original evidence");
      assert.equal(semantic.checkerVerdict, "unsure");
      assert(semantic.unsureCause, "Offline semantic finding must explain why it is unsure");
      assert.equal(semantic.requiredReview, true, "Unsure findings must require review");
      assert.equal(
        queue.reviewPending,
        queue.items.filter((finding) => finding.requiredReview && !finding.decision).length,
        "Pending count must match undecided required reviews",
      );
    } else {
      assert.equal(queue.items.length, 0, "Non-AC submissions must bypass constraint checking");
    }
    await request(`/v2/submissions/${submission.id}`, { session: otherTA, expected: 404 });
    const otherStudent = await login(item.student === "student1" ? "student2" : "student1");
    await request(`/v2/submissions/${submission.id}`, { session: otherStudent, expected: 404 });
    await request(`/v2/labs/${lab.id}/reviews`, { session: owner, expected: 404 });
    const { data: detail } = await request(`/v2/labs/${lab.id}`, { session: owner });
    assert.equal(detail.sampleTests.length, 1, "Hidden tests leaked to student lab view");
    assert(!("tests" in detail) && !("findings" in detail), "Private lab data leaked");
    report.checks.push({
      scenario: item.key,
      judge: item.judge,
      hardRule: item.hardRule ?? null,
      privacy: "passed",
    });
    console.log(`PASS ${item.key}: ${item.judge}, review and privacy checks`);
  }
  await request(`/v2/labs/${state.labs["other-class"].id}`, { session: student, expected: 404 });
  await request(`/v2/labs/${state.labs.closed.id}/submissions`, {
    method: "POST",
    session: student,
    key: "e2e-closed-probe",
    body: { source: "int main(){}" },
    expected: 409,
  });
  const admin = await login("admin");
  await request(`/v2/labs/${state.labs.compliant.id}/reviews`, { session: admin, expected: 404 });
  const { data: empty } = await request(`/v2/labs/${state.labs.empty.id}/reviews`, { session: ta });
  assert.equal(empty.items.length, 0);
  assert.equal(empty.reviewPending, 0);
  await request("/v2/auth/logout", {
    method: "POST",
    session: student,
    expected: 204,
    body: undefined,
  });
  await request("/v2/auth/me", { session: student, expected: 401 });
  report.finishedAt = new Date().toISOString();
  report.status = "passed";
  await writeFile(join(stateDir, "report.json"), JSON.stringify(report, null, 2) + "\n");
  console.log(
    `PASS role/class isolation, expired deadline and empty queue. Report: ${stateLabel}/report.json`,
  );
}

const command = process.argv[2] ?? "seed";
try {
  assert(["seed", "smoke"].includes(command), "Usage: e2e.mjs [seed|smoke] [--resubmit]");
  await load(command === "seed");
  if (command === "seed") await seed();
  else await smoke();
  if (command === "seed")
    console.log(
      `Ready: ${origin()}/live\nSynthetic passwords and lab links: ${stateLabel}/state.json`,
    );
} catch (error) {
  if (command === "smoke") {
    await writeFile(
      join(stateDir, "report.json"),
      JSON.stringify(
        { status: "failed", error: error.message, finishedAt: new Date().toISOString() },
        null,
        2,
      ) + "\n",
    ).catch(() => {});
  }
  console.error(`Local ${command} failed: ${error.message}`);
  process.exitCode = 1;
}
