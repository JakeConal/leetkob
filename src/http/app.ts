import type { IncomingMessage, ServerResponse } from "node:http";

import { verifyAuthorizationHeader } from "../auth/token.js";
import { problemResponse } from "../domain/problems.js";
import type { PublicationService } from "../domain/publication-service.js";
import type { StoredResponse } from "../domain/types.js";

export interface ApplicationDependencies {
  readonly publicationService: PublicationService;
  readonly authSecret: string;
  readonly nowSeconds?: () => number;
}

const publishPath = /^\/labs\/([^/]+)\/publish$/;
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const idempotencyKeyPattern = /^[\x21-\x7e]{8,128}$/;

function writeResponse(response: ServerResponse, result: StoredResponse): void {
  const body = JSON.stringify(result.body);
  response.writeHead(result.status, {
    "cache-control": "no-store",
    "content-length": Buffer.byteLength(body, "utf8"),
    "content-type": `${result.contentType}; charset=utf-8`,
  });
  response.end(body);
}

async function hasRequestBody(request: IncomingMessage): Promise<boolean> {
  const declaredLength = request.headers["content-length"];
  if (declaredLength !== undefined && declaredLength !== "0") {
    request.resume();
    return true;
  }

  return new Promise<boolean>((resolve, reject) => {
    let settled = false;
    const finish = (result: boolean): void => {
      if (settled) return;
      settled = true;
      resolve(result);
    };

    request.once("error", reject);
    request.once("end", () => finish(false));
    request.once("data", (chunk: Buffer | string) => {
      if (Buffer.byteLength(chunk) > 0) {
        request.resume();
        finish(true);
      }
    });
  });
}

function headerValue(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" ? value : undefined;
}

export function createRequestHandler(
  dependencies: ApplicationDependencies,
): (request: IncomingMessage, response: ServerResponse) => void {
  return (request, response) => {
    void handleRequest(request, response, dependencies).catch(() => {
      if (response.headersSent) {
        response.end();
        return;
      }
      const instance = request.url?.split("?", 1)[0] ?? "/";
      writeResponse(
        response,
        problemResponse(
          "internal-error",
          500,
          "Internal server error",
          "The request could not be completed.",
          instance,
        ),
      );
    });
  };
}

async function handleRequest(
  request: IncomingMessage,
  response: ServerResponse,
  dependencies: ApplicationDependencies,
): Promise<void> {
  const url = new URL(request.url ?? "/", "http://localhost");
  const match = publishPath.exec(url.pathname);
  if (request.method !== "POST" || match === null || url.search !== "") {
    writeResponse(
      response,
      problemResponse(
        "route-not-found",
        404,
        "Route not found",
        "No route matches this request.",
        url.pathname,
      ),
    );
    return;
  }

  const identity = verifyAuthorizationHeader(
    headerValue(request.headers.authorization),
    dependencies.authSecret,
    dependencies.nowSeconds?.(),
  );
  if (identity === undefined) {
    request.resume();
    writeResponse(
      response,
      problemResponse(
        "authentication-required",
        401,
        "Authentication required",
        "Provide a valid, unexpired bearer token.",
        url.pathname,
      ),
    );
    return;
  }

  const labId = match[1];
  const idempotencyKey = headerValue(request.headers["idempotency-key"]);
  const bodyPresent = await hasRequestBody(request);
  if (
    labId === undefined ||
    !uuidPattern.test(labId) ||
    idempotencyKey === undefined ||
    !idempotencyKeyPattern.test(idempotencyKey) ||
    bodyPresent
  ) {
    writeResponse(
      response,
      problemResponse(
        "invalid-request",
        400,
        "Invalid request",
        "Use a UUID labId, an 8-128 character visible-ASCII Idempotency-Key, and an empty body.",
        url.pathname,
      ),
    );
    return;
  }

  const result = await dependencies.publicationService.publish({
    identity,
    labId,
    idempotencyKey,
    instance: url.pathname,
  });
  writeResponse(response, result);
}
