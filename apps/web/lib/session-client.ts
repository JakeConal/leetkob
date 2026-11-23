/**
 * Session-aware client for the proxied PA#3 API.
 *
 * Every call is same-origin (`/v2/*`) so the browser attaches the HttpOnly
 * session cookie while JavaScript never sees it. Mutations additionally send
 * the in-memory CSRF proof. A 401 always clears the in-memory session and the
 * protected cache before the error reaches the screen.
 */

import { apiRequest, ApiError, createIdempotencyKey } from "./api-client";
import type {
  CurrentUser,
  Lab,
  LabCreateRequest,
  LabDetail,
  Paginated,
  ClassSummary,
  UserSession,
} from "./api-types";
import {
  API_ROUTES,
  parseClassList,
  parseCurrentUser,
  parseLab,
  parseLabDetail,
  parseUserSession,
} from "./api-types";
import {
  adoptCurrentUser,
  endSession,
  getSession,
  type ProtectedSession,
  startSession,
} from "./protected-state";

export class SessionProofMissingError extends Error {
  constructor(message = "Sign in again to restore the in-memory session proof.") {
    super(message);
    this.name = "SessionProofMissingError";
  }
}

export class AccountChangedError extends Error {
  constructor() {
    super("The account signed in on this browser changed. Sign in again.");
    this.name = "AccountChangedError";
  }
}

async function authenticatedRequest<T>(
  path: string,
  options: {
    readonly method?: "GET" | "POST";
    readonly body?: unknown;
    readonly csrfToken?: string;
    readonly idempotencyKey?: string;
    readonly parse?: (value: unknown) => T;
  },
  fetcher: typeof fetch,
): Promise<T> {
  try {
    return await apiRequest<T>(path, { ...options, fetcher });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      endSession("authentication-required");
    }
    throw error;
  }
}

export async function signIn(
  email: string,
  password: string,
  fetcher: typeof fetch = fetch,
): Promise<ProtectedSession> {
  const session = await apiRequest<UserSession>(API_ROUTES.login, {
    method: "POST",
    body: { email, password },
    parse: parseUserSession,
    fetcher,
  });
  startSession(session);
  const stored = getSession();
  if (stored === null) throw new Error("Sign-in did not establish an in-memory session");
  return stored;
}

/** Requires the in-memory CSRF proof; the cookie alone cannot sign out. */
export async function signOut(fetcher: typeof fetch = fetch): Promise<void> {
  const session = getSession();
  if (session === null) throw new SessionProofMissingError();
  try {
    await apiRequest(API_ROUTES.logout, {
      method: "POST",
      csrfToken: session.csrfToken,
      fetcher,
    });
  } finally {
    endSession("logout");
  }
}

export async function fetchCurrentUser(fetcher: typeof fetch = fetch): Promise<CurrentUser> {
  const current = await authenticatedRequest<CurrentUser>(
    API_ROUTES.me,
    { parse: parseCurrentUser },
    fetcher,
  );
  if (adoptCurrentUser(current.user) === "account-changed") {
    throw new AccountChangedError();
  }
  return current;
}

export interface FetchClassesOptions {
  readonly cursor?: string;
  readonly fetcher?: typeof fetch;
}

export async function fetchClasses(
  options: FetchClassesOptions = {},
): Promise<Paginated<ClassSummary>> {
  const fetcher = options.fetcher ?? fetch;
  const query = new URLSearchParams({ limit: "100" });
  if (options.cursor !== undefined) query.set("cursor", options.cursor);
  const page = await authenticatedRequest<Paginated<ClassSummary>>(
    `${API_ROUTES.classes}?${query.toString()}`,
    { parse: parseClassList },
    fetcher,
  );
  return page;
}

export async function fetchLab(labId: string, fetcher: typeof fetch = fetch): Promise<LabDetail> {
  return authenticatedRequest<LabDetail>(API_ROUTES.lab(labId), { parse: parseLabDetail }, fetcher);
}

export interface CreateLabOptions {
  readonly idempotencyKey?: string;
  readonly fetcher?: typeof fetch;
}

export async function createLab(
  classId: string,
  request: LabCreateRequest,
  options: CreateLabOptions = {},
): Promise<Lab> {
  const session = getSession();
  if (session === null) throw new SessionProofMissingError();
  return authenticatedRequest<Lab>(
    API_ROUTES.classLabs(classId),
    {
      method: "POST",
      body: request,
      csrfToken: session.csrfToken,
      idempotencyKey: options.idempotencyKey ?? createIdempotencyKey(),
      parse: parseLab,
    },
    options.fetcher ?? fetch,
  );
}
