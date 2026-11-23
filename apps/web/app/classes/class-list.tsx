"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";

import { summarizeError, type ApiErrorSummary } from "../../lib/api-client";
import type { ClassSummary, Paginated, User } from "../../lib/api-types";
import { formatTimestamp } from "../../lib/format";
import {
  cacheProtected,
  getLastEndReason,
  getSession,
  readProtected,
  subscribeProtectedState,
  type SessionEndReason,
} from "../../lib/protected-state";
import {
  AccountChangedError,
  fetchClasses,
  fetchCurrentUser,
  signOut,
} from "../../lib/session-client";

type ClassCache = Paginated<ClassSummary>;

type ViewState =
  | { readonly status: "signed-out"; readonly reason: SessionEndReason | null }
  | { readonly status: "loading" }
  | {
      readonly status: "ready";
      readonly user: User;
      readonly classes: readonly ClassSummary[];
      readonly nextCursor: string | null;
      readonly notice: string | null;
    }
  | { readonly status: "error"; readonly problem: ApiErrorSummary };

function signedOutMessage(reason: SessionEndReason | null): string {
  if (reason === "account-change") {
    return "The account signed in on this browser changed. Protected class data was cleared.";
  }
  if (reason === "authentication-required") {
    return "The session expired or was revoked. Sign in again.";
  }
  if (reason === "logout") {
    return "You signed out. Protected class data was cleared.";
  }
  return "The session proof is kept in memory only. Sign in again to load your classes.";
}

export function ClassList() {
  const session = useSyncExternalStore(subscribeProtectedState, getSession, () => null);
  const sessionUserId = session?.user.id ?? null;
  const sessionCsrfToken = session?.csrfToken ?? null;
  const [view, setView] = useState<ViewState>({ status: "signed-out", reason: null });
  const [reloadKey, setReloadKey] = useState(0);
  const [signOutPending, setSignOutPending] = useState(false);
  const [signOutProblem, setSignOutProblem] = useState<ApiErrorSummary | null>(null);
  const [morePending, setMorePending] = useState(false);

  useEffect(() => {
    if (sessionUserId === null || sessionCsrfToken === null) {
      setView({ status: "signed-out", reason: getLastEndReason() });
      return;
    }

    let active = true;
    const cached = readProtected<ClassCache>("classes");
    if (cached !== null) {
      const current = getSession();
      if (current !== null) {
        setView({
          status: "ready",
          user: current.user,
          classes: cached.items,
          nextCursor: cached.nextCursor,
          notice: "Showing cached class data while refreshing.",
        });
      }
    } else {
      setView({ status: "loading" });
    }

    void (async () => {
      try {
        const current = await fetchCurrentUser();
        const page = await fetchClasses();
        if (!active) return;
        cacheProtected("classes", page);
        setView({
          status: "ready",
          user: current.user,
          classes: page.items,
          nextCursor: page.nextCursor,
          notice: null,
        });
      } catch (error) {
        if (!active) return;
        if (error instanceof AccountChangedError) {
          setView({ status: "signed-out", reason: "account-change" });
          return;
        }
        setView({ status: "error", problem: summarizeError(error) });
      }
    })();

    return () => {
      active = false;
    };
  }, [sessionUserId, sessionCsrfToken, reloadKey]);

  async function handleLoadMore(): Promise<void> {
    if (view.status !== "ready" || view.nextCursor === null) return;
    setMorePending(true);
    try {
      const page = await fetchClasses({ cursor: view.nextCursor });
      const merged: ClassCache = {
        items: [...view.classes, ...page.items],
        nextCursor: page.nextCursor,
      };
      cacheProtected("classes", merged);
      setView({ ...view, classes: merged.items, nextCursor: merged.nextCursor, notice: null });
    } catch (error) {
      setView({ status: "error", problem: summarizeError(error) });
    } finally {
      setMorePending(false);
    }
  }

  async function handleSignOut(): Promise<void> {
    setSignOutPending(true);
    setSignOutProblem(null);
    try {
      await signOut();
    } catch (error) {
      setSignOutProblem(summarizeError(error));
    } finally {
      setSignOutPending(false);
    }
  }

  if (view.status === "signed-out") {
    return (
      <section className="panel" aria-labelledby="signed-out-heading">
        <div className="panel-heading">
          <h2 id="signed-out-heading">Signed out</h2>
          <p>{signedOutMessage(view.reason)}</p>
        </div>
        <div className="actions">
          <Link className="primary-action" href="/sign-in">
            Sign in
          </Link>
        </div>
      </section>
    );
  }

  if (view.status === "loading") {
    return (
      <section className="panel" aria-labelledby="loading-heading">
        <div className="panel-heading">
          <h2 id="loading-heading">Loading classes</h2>
          <p role="status">Reading the current account and its memberships…</p>
        </div>
      </section>
    );
  }

  if (view.status === "error") {
    return (
      <section className="panel" aria-labelledby="error-heading">
        <div className="panel-heading">
          <h2 id="error-heading">Classes unavailable</h2>
        </div>
        <div className="problem-alert" role="alert">
          <p className="problem-alert-title">{view.problem.title}</p>
          <p>{view.problem.detail}</p>
          <p className="problem-meta">
            {view.problem.status === null ? null : <span>HTTP {view.problem.status}</span>}
            {view.problem.type === null ? null : <span>{view.problem.type}</span>}
          </p>
        </div>
        <div className="actions">
          <button
            className="secondary-action"
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
          >
            Retry
          </button>
          <Link className="inline-action" href="/sign-in">
            Sign in again
          </Link>
        </div>
      </section>
    );
  }

  const activeSession = getSession();
  const canAuthor = view.user.role === "ta";

  return (
    <>
      <section className="panel" aria-labelledby="account-heading">
        <div className="panel-heading">
          <p className="panel-label">Current account</p>
          <h2 id="account-heading">{view.user.displayName}</h2>
          <p>
            Role: <strong>{view.user.role}</strong>
            {activeSession === null ? null : (
              <> · Session expires {formatTimestamp(activeSession.expiresAt)}</>
            )}
          </p>
        </div>

        {view.notice === null ? null : (
          <p className="status-line" role="status">
            {view.notice}
          </p>
        )}

        <div className="actions">
          <button
            className="secondary-action"
            type="button"
            onClick={() => void handleSignOut()}
            disabled={signOutPending}
          >
            {signOutPending ? "Signing out…" : "Sign out"}
          </button>
          {canAuthor ? null : (
            <span className="inline-note">
              Only an assigned TA can create labs for these classes.
            </span>
          )}
        </div>

        {signOutProblem === null ? null : (
          <div className="problem-alert" role="alert">
            <p className="problem-alert-title">{signOutProblem.title}</p>
            <p>{signOutProblem.detail}</p>
          </div>
        )}
      </section>

      <section className="panel" aria-labelledby="list-heading">
        <div className="panel-heading">
          <p className="panel-label">Memberships</p>
          <h2 id="list-heading">Classes</h2>
          <p>
            {view.classes.length === 0
              ? "No class memberships are recorded for this account."
              : `${view.classes.length} current ${view.classes.length === 1 ? "membership" : "memberships"}.`}
          </p>
        </div>

        <ul className="class-list">
          {view.classes.map((classSummary) => (
            <li className="class-row" key={classSummary.id}>
              <span className="class-name">{classSummary.name}</span>
              <span className="class-id">{classSummary.id}</span>
              {canAuthor ? (
                <Link
                  className="inline-action class-action"
                  href={`/classes/${classSummary.id}/labs/new`}
                >
                  Create lab
                </Link>
              ) : null}
            </li>
          ))}
        </ul>

        {view.nextCursor === null ? null : (
          <div className="actions">
            <button
              className="secondary-action"
              type="button"
              onClick={() => void handleLoadMore()}
              disabled={morePending}
            >
              {morePending ? "Loading…" : "Load more classes"}
            </button>
          </div>
        )}
      </section>
    </>
  );
}
