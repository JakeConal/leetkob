import type { ProblemDetails, ProblemResponse } from "./types.js";

const problemBase = "https://leetkob.example/problems";

export type ProblemCode =
  | "authentication-required"
  | "internal-error"
  | "invalid-request"
  | "lab-already-published"
  | "lab-not-found"
  | "lab-still-open"
  | "reviews-pending"
  | "route-not-found"
  | "ta-role-required";

export function problemResponse(
  code: ProblemCode,
  status: number,
  title: string,
  detail: string,
  instance: string,
  extensions: Readonly<Record<string, unknown>> = {},
): ProblemResponse {
  const body: ProblemDetails = {
    type: `${problemBase}/${code}`,
    title,
    status,
    detail,
    instance,
    ...extensions,
  };

  return { status, contentType: "application/problem+json", body };
}
