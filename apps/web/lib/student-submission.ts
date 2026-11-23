/**
 * Student submission client for the proxied PA#3 API.
 *
 * The parser is a positive allowlist: only the submission identity, state,
 * receipt time and per-test verdicts/timings ever reach the screen. Hidden
 * inputs, expected outputs, checker findings and deductions have no field in
 * these types, and unknown response members are ignored.
 */

import { apiRequest, createIdempotencyKey } from "./api-client";

export const SUBMISSION_STATES = ["queued", "judging", "judged"] as const;
export type SubmissionState = (typeof SUBMISSION_STATES)[number];

export const VERDICTS = ["AC", "WA", "TLE", "CE", "RE", "MLE"] as const;
export type Verdict = (typeof VERDICTS)[number];

export interface JudgeTestResult {
  readonly id: string;
  readonly verdict: Verdict;
  readonly timeMs: number;
}

export interface Judge {
  readonly verdict: Verdict;
  readonly tests: readonly JudgeTestResult[];
}

export interface Submission {
  readonly id: string;
  readonly labId: string;
  readonly state: SubmissionState;
  readonly receivedAt: string;
  /** Absent until the submission is judged. */
  readonly judge?: Judge;
}

export interface SubmissionAccept {
  readonly id: string;
  readonly labId: string;
  readonly state: "queued";
  readonly receivedAt: string;
}

export const submissionPath = (submissionId: string): string =>
  `/v2/submissions/${encodeURIComponent(submissionId)}`;

export const labSubmissionsPath = (labId: string): string =>
  `/v2/labs/${encodeURIComponent(labId)}/submissions`;

type JsonRecord = Readonly<Record<string, unknown>>;

function asRecord(value: unknown, context: string): JsonRecord {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${context}: expected an object`);
  }
  return value as JsonRecord;
}

function asString(record: JsonRecord, key: string, context: string): string {
  const value = record[key];
  if (typeof value !== "string") {
    throw new Error(`${context}: "${key}" must be a string`);
  }
  return value;
}

function asInteger(record: JsonRecord, key: string, context: string): number {
  const value = record[key];
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0) {
    throw new Error(`${context}: "${key}" must be a non-negative integer`);
  }
  return value;
}

function asArray(record: JsonRecord, key: string, context: string): readonly unknown[] {
  const value = record[key];
  if (!Array.isArray(value)) {
    throw new Error(`${context}: "${key}" must be an array`);
  }
  return value;
}

function parseState(value: unknown, context: string): SubmissionState {
  if (typeof value === "string" && (SUBMISSION_STATES as readonly string[]).includes(value)) {
    return value as SubmissionState;
  }
  throw new Error(`${context}: state must be one of ${SUBMISSION_STATES.join(", ")}`);
}

function parseVerdict(value: unknown, context: string): Verdict {
  if (typeof value === "string" && (VERDICTS as readonly string[]).includes(value)) {
    return value as Verdict;
  }
  throw new Error(`${context}: verdict must be one of ${VERDICTS.join(", ")}`);
}

export function parseJudgeTestResult(value: unknown, context = "judge.tests[]"): JudgeTestResult {
  const record = asRecord(value, context);
  return {
    id: asString(record, "id", context),
    verdict: parseVerdict(record["verdict"], context),
    timeMs: asInteger(record, "timeMs", context),
  };
}

export function parseJudge(value: unknown, context = "judge"): Judge {
  const record = asRecord(value, context);
  return {
    verdict: parseVerdict(record["verdict"], context),
    tests: asArray(record, "tests", context).map((item) =>
      parseJudgeTestResult(item, `${context}.tests[]`),
    ),
  };
}

export function parseSubmission(value: unknown): Submission {
  const record = asRecord(value, "submission");
  const base: Submission = {
    id: asString(record, "id", "submission"),
    labId: asString(record, "labId", "submission"),
    state: parseState(record["state"], "submission"),
    receivedAt: asString(record, "receivedAt", "submission"),
  };
  const judge = record["judge"];
  if (base.state === "judged") {
    if (judge === undefined) {
      throw new Error("submission: a judged submission must include judge");
    }
    return { ...base, judge: parseJudge(judge) };
  }
  if (judge !== undefined) {
    throw new Error("submission: judge must be absent until judged");
  }
  return base;
}

export function parseSubmissionAccept(value: unknown): SubmissionAccept {
  const record = asRecord(value, "submission-accept");
  const state = asString(record, "state", "submission-accept");
  if (state !== "queued") {
    throw new Error('submission-accept: state must be "queued"');
  }
  return {
    id: asString(record, "id", "submission-accept"),
    labId: asString(record, "labId", "submission-accept"),
    state,
    receivedAt: asString(record, "receivedAt", "submission-accept"),
  };
}

export interface SubmitSourceOptions {
  readonly csrfToken: string;
  readonly idempotencyKey?: string;
  readonly fetcher?: typeof fetch;
  readonly signal?: AbortSignal;
}

/** Submits bounded C++17 source with the CSRF proof and an idempotency key. */
export async function submitSource(
  labId: string,
  source: string,
  options: SubmitSourceOptions,
): Promise<SubmissionAccept> {
  if (source === "") {
    throw new Error("Source is required.");
  }
  const fetcher = options.fetcher ?? fetch;
  return apiRequest<SubmissionAccept>(labSubmissionsPath(labId), {
    method: "POST",
    body: { source },
    csrfToken: options.csrfToken,
    idempotencyKey: options.idempotencyKey ?? createIdempotencyKey(),
    parse: parseSubmissionAccept,
    fetcher,
    ...(options.signal === undefined ? {} : { signal: options.signal }),
  });
}

export interface FetchSubmissionOptions {
  readonly fetcher?: typeof fetch;
  readonly signal?: AbortSignal;
}

/** Reads the owner-safe submission projection. */
export async function fetchSubmission(
  submissionId: string,
  options: FetchSubmissionOptions = {},
): Promise<Submission> {
  const fetcher = options.fetcher ?? fetch;
  return apiRequest<Submission>(submissionPath(submissionId), {
    parse: parseSubmission,
    fetcher,
    ...(options.signal === undefined ? {} : { signal: options.signal }),
  });
}

export class SubmissionPollTimeoutError extends Error {
  constructor(attempts: number) {
    super(`The submission did not reach a terminal state after ${attempts} status checks.`);
    this.name = "SubmissionPollTimeoutError";
  }
}

export interface PollSubmissionOptions extends FetchSubmissionOptions {
  readonly submissionId: string;
  readonly intervalMs?: number;
  readonly maxAttempts?: number;
  /** Injected in tests so polling never waits for real time. */
  readonly wait?: (milliseconds: number) => Promise<void>;
  readonly onUpdate?: (submission: Submission) => void;
}

/**
 * Refetches the submission until it is judged. SSE is later work, so this
 * bounded poll mirrors the documented screen behavior and stops on the first
 * terminal response.
 */
export async function pollSubmissionUntilJudged(
  options: PollSubmissionOptions,
): Promise<Submission> {
  const intervalMs = options.intervalMs ?? 1000;
  const maxAttempts = options.maxAttempts ?? 1800;
  const wait =
    options.wait ??
    ((milliseconds: number) => new Promise<void>((resolve) => setTimeout(resolve, milliseconds)));

  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const submission = await fetchSubmission(options.submissionId, {
      ...(options.fetcher === undefined ? {} : { fetcher: options.fetcher }),
      ...(options.signal === undefined ? {} : { signal: options.signal }),
    });
    options.onUpdate?.(submission);
    if (submission.state === "judged") return submission;
    if (attempt < maxAttempts) await wait(intervalMs);
  }
  throw new SubmissionPollTimeoutError(maxAttempts);
}
