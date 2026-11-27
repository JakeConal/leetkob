import { liveApi, withCursor } from "./paths";

export interface Problem {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail: string;
  readonly instance: string;
}

export class ApiProblem extends Error {
  readonly problem: Problem;

  constructor(problem: Problem) {
    super(problem.detail);
    this.name = "ApiProblem";
    this.problem = problem;
  }

  get status(): number {
    return this.problem.status;
  }
}

export class ApiConnectionError extends Error {
  constructor() {
    super("Could not reach the API. The request was not completed.");
    this.name = "ApiConnectionError";
  }
}

export class ContractError extends Error {
  constructor() {
    super("The API response did not match the contract.");
    this.name = "ContractError";
  }
}

export function isNotFound(error: unknown): boolean {
  return error instanceof ApiProblem && error.status === 404;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseProblem(value: unknown, status: number): Problem {
  if (isRecord(value) && typeof value["detail"] === "string" && value["detail"].length > 0) {
    return {
      type: typeof value["type"] === "string" ? value["type"] : "about:blank",
      title: typeof value["title"] === "string" ? value["title"] : "Request failed",
      status: typeof value["status"] === "number" ? value["status"] : status,
      detail: value["detail"],
      instance: typeof value["instance"] === "string" ? value["instance"] : "",
    };
  }
  return {
    type: "about:blank",
    title: "Request failed",
    status,
    detail: `The request failed with status ${status}.`,
    instance: "",
  };
}

export interface LiveRequest {
  readonly method: "GET" | "POST";
  readonly path: string;
  readonly body?: unknown;
  readonly csrfToken?: string;
  readonly idempotencyKey?: string;
  readonly expect: number;
}

export async function liveFetch(request: LiveRequest): Promise<unknown> {
  const headers: Record<string, string> = {
    accept: "application/json, application/problem+json",
  };
  if (request.body !== undefined) headers["content-type"] = "application/json";
  if (request.csrfToken !== undefined) headers["X-CSRF-Token"] = request.csrfToken;
  if (request.idempotencyKey !== undefined) headers["Idempotency-Key"] = request.idempotencyKey;

  let response: Response;
  try {
    response = await fetch(request.path, {
      method: request.method,
      credentials: "include",
      cache: "no-store",
      headers,
      ...(request.body !== undefined ? { body: JSON.stringify(request.body) } : {}),
    });
  } catch {
    throw new ApiConnectionError();
  }

  if (response.status === 204 && request.expect === 204) return null;

  const text = await response.text();
  let parsed: unknown = null;
  if (text.length > 0) {
    try {
      parsed = JSON.parse(text) as unknown;
    } catch {
      throw new ContractError();
    }
  }
  if (response.status !== request.expect)
    throw new ApiProblem(parseProblem(parsed, response.status));
  return parsed;
}

export interface User {
  readonly id: string;
  readonly displayName: string;
  readonly role: "admin" | "ta" | "student";
}

export interface UserSession {
  readonly user: User;
  readonly csrfToken: string;
  readonly expiresAt: string;
}

export interface ClassSummary {
  readonly id: string;
  readonly name: string;
}

export interface LabSummary {
  readonly id: string;
  readonly classId: string;
  readonly title: string;
  readonly opensAt: string;
  readonly deadline: string;
  readonly revision: number;
}

export interface Evidence {
  readonly line: number;
  readonly text: string;
}

export interface Finding {
  readonly id: string;
  readonly submissionId: string;
  readonly labId: string;
  readonly constraintId: string;
  readonly constraintRevision: number;
  readonly revision?: number;
  readonly source: string;
  readonly checkerVerdict: "pass" | "violation" | "unsure";
  readonly unsureCause?: string;
  readonly evidence: readonly Evidence[];
  readonly deduction: number;
  readonly requiredReview: boolean;
  readonly decision?: { readonly verdict: "pass" | "violation"; readonly reason: string };
}

export interface ReviewQueue {
  readonly items: readonly Finding[];
  readonly nextCursor: string | null;
  readonly processingPending: boolean;
  readonly reviewPending: number;
}

export interface SubmissionAccept {
  readonly id: string;
  readonly labId: string;
  readonly state: "queued";
  readonly receivedAt: string;
}

export interface CreatedLab {
  readonly id: string;
  readonly classId: string;
  readonly title: string;
}

function expectString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== "string" || value.length === 0) throw new ContractError();
  return value;
}

function expectRecord(value: unknown): Record<string, unknown> {
  if (!isRecord(value)) throw new ContractError();
  return value;
}

const ROLES = new Set(["admin", "ta", "student"]);

export function parseUser(value: unknown): User {
  const record = expectRecord(value);
  const role = record["role"];
  if (typeof role !== "string" || !ROLES.has(role)) throw new ContractError();
  return {
    id: expectString(record, "id"),
    displayName: expectString(record, "displayName"),
    role: role as User["role"],
  };
}

export function parseUserSession(value: unknown): UserSession {
  const record = expectRecord(value);
  return {
    user: parseUser(record["user"]),
    csrfToken: expectString(record, "csrfToken"),
    expiresAt: expectString(record, "expiresAt"),
  };
}

function parseCursor(value: unknown): string | null {
  if (value === null) return null;
  if (typeof value === "string") return value;
  throw new ContractError();
}

export function parseClassList(value: unknown): {
  items: ClassSummary[];
  nextCursor: string | null;
} {
  const record = expectRecord(value);
  if (!Array.isArray(record["items"])) throw new ContractError();
  const items = record["items"].map((item) => {
    const row = expectRecord(item);
    return { id: expectString(row, "id"), name: expectString(row, "name") };
  });
  return { items, nextCursor: parseCursor(record["nextCursor"]) };
}

export function parseLabList(value: unknown): { items: LabSummary[]; nextCursor: string | null } {
  const record = expectRecord(value);
  if (!Array.isArray(record["items"])) throw new ContractError();
  const items = record["items"].map((item) => {
    const row = expectRecord(item);
    const revision = row["revision"];
    if (typeof revision !== "number") throw new ContractError();
    return {
      id: expectString(row, "id"),
      classId: expectString(row, "classId"),
      title: expectString(row, "title"),
      opensAt: expectString(row, "opensAt"),
      deadline: expectString(row, "deadline"),
      revision,
    };
  });
  return { items, nextCursor: parseCursor(record["nextCursor"]) };
}

export function parseCreatedLab(value: unknown): CreatedLab {
  const record = expectRecord(value);
  if (typeof record["modelNotice"] !== "string" || record["modelNotice"].length === 0) {
    throw new ContractError();
  }
  return {
    id: expectString(record, "id"),
    classId: expectString(record, "classId"),
    title: expectString(record, "title"),
  };
}

export function parseSubmissionAccept(value: unknown): SubmissionAccept {
  const record = expectRecord(value);
  if (record["state"] !== "queued") throw new ContractError();
  return {
    id: expectString(record, "id"),
    labId: expectString(record, "labId"),
    state: "queued",
    receivedAt: expectString(record, "receivedAt"),
  };
}

const CHECKER_VERDICTS = new Set(["pass", "violation", "unsure"]);

function parseEvidence(value: unknown): Evidence {
  const record = expectRecord(value);
  const line = record["line"];
  if (typeof line !== "number" || !Number.isInteger(line) || line < 1) throw new ContractError();
  return { line, text: expectString(record, "text") };
}

function parseFinding(value: unknown): Finding {
  const record = expectRecord(value);
  const checkerVerdict = record["checkerVerdict"];
  if (typeof checkerVerdict !== "string" || !CHECKER_VERDICTS.has(checkerVerdict)) {
    throw new ContractError();
  }
  const constraintRevision = record["constraintRevision"];
  const deduction = record["deduction"];
  if (typeof constraintRevision !== "number" || typeof deduction !== "number")
    throw new ContractError();
  if (typeof record["source"] !== "string" || !Array.isArray(record["evidence"]))
    throw new ContractError();
  if (typeof record["requiredReview"] !== "boolean") throw new ContractError();
  const revision = record["revision"];
  const unsureCause = record["unsureCause"];
  const decision = record["decision"];
  let parsedDecision: Finding["decision"];
  if (decision !== undefined && decision !== null) {
    const row = expectRecord(decision);
    const verdict = row["verdict"];
    if (verdict !== "pass" && verdict !== "violation") throw new ContractError();
    parsedDecision = { verdict, reason: expectString(row, "reason") };
  }
  return {
    id: expectString(record, "id"),
    submissionId: expectString(record, "submissionId"),
    labId: expectString(record, "labId"),
    constraintId: expectString(record, "constraintId"),
    constraintRevision,
    ...(typeof revision === "number" && Number.isInteger(revision) && revision >= 1
      ? { revision }
      : {}),
    source: record["source"],
    checkerVerdict: checkerVerdict as Finding["checkerVerdict"],
    ...(typeof unsureCause === "string" && unsureCause.length > 0 ? { unsureCause } : {}),
    evidence: record["evidence"].map((item) => parseEvidence(item)),
    deduction,
    requiredReview: record["requiredReview"],
    ...(parsedDecision !== undefined ? { decision: parsedDecision } : {}),
  };
}

export function parseReviewQueue(value: unknown): ReviewQueue {
  const record = expectRecord(value);
  if (!Array.isArray(record["items"])) throw new ContractError();
  const reviewPending = record["reviewPending"];
  if (typeof reviewPending !== "number" || !Number.isInteger(reviewPending) || reviewPending < 0) {
    throw new ContractError();
  }
  if (typeof record["processingPending"] !== "boolean") throw new ContractError();
  return {
    items: record["items"].map((item) => parseFinding(item)),
    nextCursor: parseCursor(record["nextCursor"]),
    processingPending: record["processingPending"],
    reviewPending,
  };
}

export function expectedFindingRevision(
  finding: Pick<Finding, "revision" | "constraintRevision">,
): number {
  return finding.revision ?? finding.constraintRevision;
}

export async function login(email: string, password: string): Promise<UserSession> {
  const body = await liveFetch({
    method: "POST",
    path: liveApi.login,
    body: { email, password },
    expect: 200,
  });
  return parseUserSession(body);
}

export async function logout(csrfToken: string): Promise<void> {
  await liveFetch({ method: "POST", path: liveApi.logout, csrfToken, expect: 204 });
}

export async function getCurrentUser(): Promise<{ user: User; classes: ClassSummary[] }> {
  const body = await liveFetch({ method: "GET", path: liveApi.me, expect: 200 });
  const record = expectRecord(body);
  if (!Array.isArray(record["classes"])) throw new ContractError();
  return {
    user: parseUser(record["user"]),
    classes: record["classes"].map((item) => {
      const row = expectRecord(item);
      return { id: expectString(row, "id"), name: expectString(row, "name") };
    }),
  };
}

export async function listClasses(): Promise<{ items: ClassSummary[]; nextCursor: string | null }> {
  return parseClassList(await liveFetch({ method: "GET", path: liveApi.classes, expect: 200 }));
}

export async function listLabs(
  classId: string,
  cursor: string | null = null,
): Promise<{ items: LabSummary[]; nextCursor: string | null }> {
  return parseLabList(
    await liveFetch({
      method: "GET",
      path: withCursor(liveApi.classLabs(classId), cursor),
      expect: 200,
    }),
  );
}

export async function readLab(labId: string): Promise<unknown> {
  return liveFetch({ method: "GET", path: liveApi.lab(labId), expect: 200 });
}

export async function createLab(
  classId: string,
  body: unknown,
  csrfToken: string,
  idempotencyKey: string,
): Promise<CreatedLab> {
  return parseCreatedLab(
    await liveFetch({
      method: "POST",
      path: liveApi.classLabs(classId),
      body,
      csrfToken,
      idempotencyKey,
      expect: 201,
    }),
  );
}

export async function submitSource(
  labId: string,
  source: string,
  csrfToken: string,
  idempotencyKey: string,
): Promise<SubmissionAccept> {
  return parseSubmissionAccept(
    await liveFetch({
      method: "POST",
      path: liveApi.submissions(labId),
      body: { source },
      csrfToken,
      idempotencyKey,
      expect: 202,
    }),
  );
}

export async function readSubmission(submissionId: string): Promise<unknown> {
  return liveFetch({ method: "GET", path: liveApi.submission(submissionId), expect: 200 });
}

export async function listReviews(labId: string, cursor: string | null): Promise<ReviewQueue> {
  return parseReviewQueue(
    await liveFetch({
      method: "GET",
      path: withCursor(liveApi.reviews(labId), cursor),
      expect: 200,
    }),
  );
}

export async function decideFinding(
  labId: string,
  findingId: string,
  body: unknown,
  csrfToken: string,
  idempotencyKey: string,
): Promise<Finding> {
  return parseFinding(
    await liveFetch({
      method: "POST",
      path: liveApi.decision(labId, findingId),
      body,
      csrfToken,
      idempotencyKey,
      expect: 200,
    }),
  );
}

export function deadlineHasPassed(deadline: string, now: number): boolean {
  const time = Date.parse(deadline);
  if (Number.isNaN(time)) return false;
  return now >= time;
}
