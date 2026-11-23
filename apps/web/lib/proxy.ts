/**
 * Same-origin proxy for exactly the PA#3 `/v2` routes the browser uses.
 *
 * The HttpOnly session cookie belongs to the Next.js origin, so browser code
 * never reads it and never sees the Go API origin. Only an allowlist of
 * routes, methods, headers and response headers crosses the boundary, and
 * every response is `no-store`.
 */

export const DEFAULT_GO_API_URL = "http://127.0.0.1:8080";

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const FORWARDED_REQUEST_HEADERS = [
  "accept",
  "content-type",
  "cookie",
  "origin",
  "x-csrf-token",
  "idempotency-key",
] as const;

const FORWARDED_RESPONSE_HEADERS = ["content-type", "retry-after", "www-authenticate"] as const;

function goApiBaseUrl(): string {
  const configured = process.env["GO_API_URL"];
  return configured === undefined || configured === "" ? DEFAULT_GO_API_URL : configured;
}

export function isUuid(value: string): boolean {
  return UUID_PATTERN.test(value);
}

/** Only the documented PA#3 browser routes are proxied. */
export function isAllowedV2Request(method: string, segments: readonly string[]): boolean {
  const upperMethod = method.toUpperCase();
  if (segments.length === 0 || segments.some((segment) => segment === "")) return false;

  const [first, second, third, ...rest] = segments;
  if (rest.length > 0) return false;

  if (first === "auth" && second === "me" && third === undefined) {
    return upperMethod === "GET";
  }
  if (first === "auth" && second === "login" && third === undefined) {
    return upperMethod === "POST";
  }
  if (first === "auth" && second === "logout" && third === undefined) {
    return upperMethod === "POST";
  }
  if (first === "classes" && second === undefined) {
    return upperMethod === "GET";
  }
  if (first === "classes" && second !== undefined && third === "labs") {
    return isUuid(second) && (upperMethod === "GET" || upperMethod === "POST");
  }
  if (first === "labs" && second !== undefined && third === undefined) {
    return isUuid(second) && upperMethod === "GET";
  }
  if (first === "labs" && second !== undefined && third === "submissions") {
    return isUuid(second) && (upperMethod === "GET" || upperMethod === "POST");
  }
  if (first === "submissions" && second !== undefined && third === undefined) {
    return isUuid(second) && upperMethod === "GET";
  }
  return false;
}

function problemResponse(
  status: number,
  type: string,
  title: string,
  detail: string,
  instance: string,
): Response {
  return new Response(
    JSON.stringify({
      type: `https://leetkob.example/problems/${type}`,
      title,
      status,
      detail,
      instance,
    }),
    {
      status,
      headers: {
        "content-type": "application/problem+json",
        "cache-control": "no-store",
      },
    },
  );
}

function requestPath(request: Request): string {
  return new URL(request.url).pathname;
}

function concealedNotFound(instance: string): Response {
  return problemResponse(
    404,
    "resource-not-found",
    "Resource not found",
    "The requested resource does not exist.",
    instance,
  );
}

export async function proxyV2Request(
  method: string,
  request: Request,
  segments: readonly string[],
): Promise<Response> {
  const instance = requestPath(request);
  if (!isAllowedV2Request(method, segments)) {
    return concealedNotFound(instance);
  }

  const incomingUrl = new URL(request.url);
  const upstreamUrl = new URL(`/v2/${segments.map(encodeURIComponent).join("/")}`, goApiBaseUrl());
  upstreamUrl.search = incomingUrl.search;

  const headers = new Headers();
  for (const name of FORWARDED_REQUEST_HEADERS) {
    const value = request.headers.get(name);
    if (value !== null) headers.set(name, value);
  }
  if (!headers.has("accept")) headers.set("accept", "application/json");

  const init: RequestInit = {
    method,
    headers,
    cache: "no-store",
    redirect: "manual",
  };
  if (method !== "GET" && method !== "HEAD") {
    init.body = await request.arrayBuffer();
  }

  let upstream: Response;
  try {
    upstream = await fetch(upstreamUrl, init);
  } catch {
    return problemResponse(
      503,
      "unavailable",
      "Service unavailable",
      "The service is temporarily unavailable. The request can be safely retried.",
      instance,
    );
  }

  const responseHeaders = new Headers();
  for (const name of FORWARDED_RESPONSE_HEADERS) {
    const value = upstream.headers.get(name);
    if (value !== null) responseHeaders.set(name, value);
  }
  for (const cookie of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", cookie);
  }
  responseHeaders.set("cache-control", "no-store");

  const body =
    upstream.status === 204 || upstream.status === 304 ? null : await upstream.arrayBuffer();
  return new Response(body, { status: upstream.status, headers: responseHeaders });
}
