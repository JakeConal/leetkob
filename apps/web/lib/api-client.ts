/**
 * Same-origin fetch wrapper for the proxied PA#3 API.
 *
 * - Parses RFC 9457 `application/problem+json` bodies into `ApiProblem`.
 * - Injects `X-CSRF-Token` for authenticated mutations only.
 * - Sends `Idempotency-Key` when supplied.
 * - Never caches: every request uses `cache: "no-store"`.
 * - Surfaces 401 as `ApiError` so the session layer can clear protected state.
 */

export interface ApiProblem {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail: string;
  readonly instance: string;
  readonly extensions: Readonly<Record<string, unknown>>;
}

export type HttpMethod = "GET" | "POST";

export class ApiError extends Error {
  readonly status: number;
  readonly problem: ApiProblem | null;
  readonly retryAfterSeconds: number | null;

  constructor(status: number, problem: ApiProblem | null, retryAfterSeconds: number | null = null) {
    super(
      problem === null
        ? `Request failed with status ${status}`
        : `${problem.title}: ${problem.detail}`,
    );
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
    this.retryAfterSeconds = retryAfterSeconds;
  }

  get isUnauthenticated(): boolean {
    return this.status === 401;
  }
}

export interface ApiErrorSummary {
  readonly title: string;
  readonly detail: string;
  readonly status: number | null;
  readonly type: string | null;
  readonly instance: string | null;
  readonly retryAfterSeconds: number | null;
}

const PROBLEM_CORE_FIELDS = ["type", "title", "status", "detail", "instance"] as const;

function isRecord(value: unknown): value is Readonly<Record<string, unknown>> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Validates the RFC 9457 core members and preserves documented extensions. */
export function parseProblemBody(value: unknown): ApiProblem | null {
  if (!isRecord(value)) return null;
  for (const field of PROBLEM_CORE_FIELDS) {
    if (!(field in value)) return null;
  }
  if (
    typeof value["type"] !== "string" ||
    typeof value["title"] !== "string" ||
    typeof value["status"] !== "number" ||
    typeof value["detail"] !== "string" ||
    typeof value["instance"] !== "string"
  ) {
    return null;
  }

  const extensions: Record<string, unknown> = {};
  for (const [key, extensionValue] of Object.entries(value)) {
    if (!(PROBLEM_CORE_FIELDS as readonly string[]).includes(key)) {
      extensions[key] = extensionValue;
    }
  }

  return {
    type: value["type"],
    title: value["title"],
    status: value["status"],
    detail: value["detail"],
    instance: value["instance"],
    extensions,
  };
}

export async function readProblem(response: Response): Promise<ApiProblem | null> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.toLowerCase().includes("json")) return null;
  try {
    return parseProblemBody((await response.json()) as unknown);
  } catch {
    return null;
  }
}

function readRetryAfter(response: Response): number | null {
  const raw = response.headers.get("retry-after");
  if (raw === null) return null;
  const seconds = Number.parseInt(raw, 10);
  return Number.isInteger(seconds) && seconds > 0 ? seconds : null;
}

export interface ApiRequestOptions<T> {
  readonly method?: HttpMethod;
  readonly body?: unknown;
  /** Session CSRF proof; injected only on mutations. */
  readonly csrfToken?: string;
  /** 8-128 visible ASCII characters, scoped to actor/operation/resource. */
  readonly idempotencyKey?: string;
  readonly parse?: (value: unknown) => T;
  readonly fetcher?: typeof fetch;
  readonly signal?: AbortSignal;
}

export async function apiRequest<T>(path: string, options: ApiRequestOptions<T> = {}): Promise<T> {
  const method = options.method ?? "GET";
  const fetcher = options.fetcher ?? fetch;
  const headers = new Headers({ accept: "application/json" });

  if (options.body !== undefined) {
    headers.set("content-type", "application/json");
  }
  if (method === "POST" && options.csrfToken !== undefined) {
    headers.set("x-csrf-token", options.csrfToken);
  }
  if (options.idempotencyKey !== undefined) {
    headers.set("idempotency-key", options.idempotencyKey);
  }

  const requestInit: RequestInit = {
    method,
    headers,
    cache: "no-store",
    credentials: "same-origin",
  };
  if (options.body !== undefined) {
    requestInit.body = JSON.stringify(options.body);
  }
  if (options.signal !== undefined) {
    requestInit.signal = options.signal;
  }

  const response = await fetcher(path, requestInit);
  if (!response.ok) {
    throw new ApiError(response.status, await readProblem(response), readRetryAfter(response));
  }
  if (options.parse === undefined) {
    return undefined as unknown as T;
  }
  return options.parse((await response.json()) as unknown);
}

/** Random 32-hex-character idempotency key (satisfies the 8-128 visible ASCII rule). */
export function createIdempotencyKey(
  randomBytes: Uint8Array = globalThis.crypto.getRandomValues(new Uint8Array(16)),
): string {
  return Array.from(randomBytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function summarizeError(error: unknown): ApiErrorSummary {
  if (error instanceof ApiError) {
    return {
      title: error.problem?.title ?? `Request failed (${error.status})`,
      detail: error.problem?.detail ?? error.message,
      status: error.status,
      type: error.problem?.type ?? null,
      instance: error.problem?.instance ?? null,
      retryAfterSeconds: error.retryAfterSeconds,
    };
  }
  if (error instanceof Error) {
    return {
      title: "Request failed",
      detail: error.message,
      status: null,
      type: null,
      instance: null,
      retryAfterSeconds: null,
    };
  }
  return {
    title: "Request failed",
    detail: "An unexpected error occurred.",
    status: null,
    type: null,
    instance: null,
    retryAfterSeconds: null,
  };
}
