/**
 * In-memory session and protected-read store.
 *
 * Browser state never touches `localStorage` or `sessionStorage`: the HttpOnly
 * session cookie stays unreadable to JavaScript, and the CSRF proof plus any
 * protected reads live only in this module for the lifetime of the page.
 * Signing out, a 401, or a different account at the same browser clears both
 * the session and every cached protected value (review-dashboard PA3-UI-02).
 */

import type { User } from "./api-types";

export type SessionEndReason = "logout" | "account-change" | "authentication-required";

export interface ProtectedSession {
  readonly user: User;
  readonly csrfToken: string;
  readonly expiresAt: string;
}

interface StoreState {
  readonly session: ProtectedSession | null;
  readonly epoch: number;
}

interface CacheEntry {
  readonly epoch: number;
  readonly value: unknown;
}

type Listener = () => void;

let state: StoreState = { session: null, epoch: 0 };
let lastEndReason: SessionEndReason | null = null;
const protectedCache = new Map<string, CacheEntry>();
const listeners = new Set<Listener>();

function notify(): void {
  for (const listener of Array.from(listeners)) {
    listener();
  }
}

function clearCache(): void {
  protectedCache.clear();
}

export function subscribeProtectedState(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getSession(): ProtectedSession | null {
  return state.session;
}

export function getCsrfToken(): string | null {
  return state.session?.csrfToken ?? null;
}

export function getLastEndReason(): SessionEndReason | null {
  return lastEndReason;
}

export function startSession(session: ProtectedSession): { readonly accountChanged: boolean } {
  const previous = state.session;
  const accountChanged = previous !== null && previous.user.id !== session.user.id;
  if (accountChanged) {
    clearCache();
  }
  state = { session, epoch: accountChanged ? state.epoch + 1 : state.epoch };
  lastEndReason = null;
  notify();
  return { accountChanged };
}

/** Clears the session and every protected value, whatever the trigger. */
export function endSession(reason: SessionEndReason): void {
  clearCache();
  lastEndReason = reason;
  if (state.session !== null) {
    state = { session: null, epoch: state.epoch + 1 };
  }
  notify();
}

/**
 * Reconciles the server-reported current user with the in-memory session.
 * A different identity means the cookie now belongs to someone else, so the
 * previous account's protected state must not survive.
 */
export function adoptCurrentUser(user: User): "kept" | "account-changed" | "signed-out" {
  const session = state.session;
  if (session === null) return "signed-out";
  if (session.user.id !== user.id) {
    endSession("account-change");
    return "account-changed";
  }
  state = { session: { ...session, user }, epoch: state.epoch };
  notify();
  return "kept";
}

/** Stores a protected read for the current account only. */
export function cacheProtected<T>(key: string, value: T): void {
  if (state.session === null) {
    throw new Error("Cannot cache protected data without an authenticated session");
  }
  protectedCache.set(key, { epoch: state.epoch, value });
}

export function readProtected<T>(key: string): T | null {
  const entry = protectedCache.get(key);
  if (entry === undefined || entry.epoch !== state.epoch) return null;
  return entry.value as T;
}

export function clearProtectedState(): void {
  clearCache();
  notify();
}

/** Test-only reset; the browser never calls this outside a full reload. */
export function resetProtectedStateForTests(): void {
  protectedCache.clear();
  state = { session: null, epoch: state.epoch + 1 };
  lastEndReason = null;
  notify();
}
