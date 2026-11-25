"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";

import { ApiProblem, getCurrentUser, listClasses, listLabs } from "../../lib/live/api";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
import { formatDateTime } from "../../lib/live/format";
import { useLiveSession } from "./live-shell";

const queryOptions = { retry: false, gcTime: 0, staleTime: 0 } as const;

function useRedirectSignedOut(error: unknown) {
  const router = useRouter();
  const signedOut = error instanceof ApiProblem && error.status === 401;
  useEffect(() => {
    if (signedOut) router.replace("/live");
  }, [router, signedOut]);
  return signedOut;
}

export function ClassList() {
  const session = useLiveSession();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const classes = useQuery({
    queryKey: ["live", "classes"],
    queryFn: listClasses,
    enabled: session.ready && !session.closing && me.isSuccess,
    ...queryOptions,
  });
  const signedOut = useRedirectSignedOut(me.error ?? classes.error);
  const notFound = useReplaceNotFound(classes.error);

  if (!session.ready || session.closing) return <p className="live-loading">Loading…</p>;
  if (signedOut || notFound)
    return notFound ? <LiveNotFound /> : <p className="live-muted">Sign in required.</p>;
  if (me.isPending || classes.isPending) return <p className="live-loading">Loading classes…</p>;
  if (me.isError) return <LiveError error={me.error} />;
  if (classes.isError) return <LiveError error={classes.error} />;
  if (!classes.data) return null;

  return (
    <section className="live-page" aria-labelledby="classes-heading">
      <header className="live-page-head">
        <div className="live-heading-group">
          <h1 id="classes-heading">Classes</h1>
          <p className="live-muted">
            {me.data.user.displayName} · {me.data.user.role}
          </p>
        </div>
      </header>
      {classes.data.items.length === 0 ? (
        <p className="live-empty">No classes are assigned to this account.</p>
      ) : (
        <ul className="live-panel live-rows">
          {classes.data.items.map((item) => (
            <li key={item.id}>
              <Link className="live-row-link" href={`/live/classes/${item.id}`}>
                <span>{item.name}</span>
                <span className="live-muted">View labs</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

export function ClassLabs({ classId }: { readonly classId: string }) {
  const session = useLiveSession();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const labs = useQuery({
    queryKey: ["live", "labs", classId],
    queryFn: () => listLabs(classId),
    enabled: session.ready && !session.closing && me.isSuccess,
    ...queryOptions,
  });
  const signedOut = useRedirectSignedOut(me.error ?? labs.error);
  const notFound = useReplaceNotFound(labs.error);

  if (!session.ready || session.closing) return <p className="live-loading">Loading…</p>;
  if (signedOut) return <p className="live-muted">Sign in required.</p>;
  if (notFound) return <LiveNotFound />;
  if (me.isPending || labs.isPending) return <p className="live-loading">Loading labs…</p>;
  if (me.isError) return <LiveError error={me.error} />;
  if (labs.isError) return <LiveError error={labs.error} />;
  if (!labs.data || !me.data) return null;

  const isTa = me.data.user.role === "ta";
  const now = Date.now();
  return (
    <section className="live-page" aria-labelledby="labs-heading">
      <nav className="live-crumbs" aria-label="Breadcrumb">
        <Link href="/live/classes">Classes</Link>
        <span aria-hidden="true">/</span>
        <span aria-current="page">Labs</span>
      </nav>
      <header className="live-page-head">
        <h1 id="labs-heading">Labs</h1>
        {isTa ? (
          <Link className="live-button" href={`/live/classes/${classId}/labs/new`}>
            Create lab
          </Link>
        ) : null}
      </header>
      {labs.data.items.length === 0 ? (
        <p className="live-empty">No labs are visible in this class.</p>
      ) : (
        <div className="live-panel live-table-wrap">
          <table className="live-table live-table--stack">
            <thead>
              <tr>
                <th scope="col">Lab</th>
                <th scope="col">Opens</th>
                <th scope="col">Deadline</th>
                <th scope="col">Status</th>
                <th scope="col" className="live-num">
                  Revision
                </th>
                {isTa ? (
                  <th scope="col">
                    <span className="live-visually-hidden">Actions</span>
                  </th>
                ) : null}
              </tr>
            </thead>
            <tbody>
              {labs.data.items.map((lab) => {
                const closed = Date.parse(lab.deadline) <= now;
                const upcoming = Date.parse(lab.opensAt) > now;
                const status = closed ? "Closed" : upcoming ? "Not open yet" : "Open";
                return (
                  <tr key={lab.id}>
                    <th scope="row" data-label="Lab">
                      <Link className="live-strong-link" href={`/live/labs/${lab.id}`}>
                        {lab.title}
                      </Link>
                    </th>
                    <td data-label="Opens">
                      <time dateTime={lab.opensAt}>{formatDateTime(lab.opensAt)}</time>
                    </td>
                    <td data-label="Deadline">
                      <time dateTime={lab.deadline}>{formatDateTime(lab.deadline)}</time>
                    </td>
                    <td data-label="Status">
                      <span
                        className={`live-status ${closed ? "" : upcoming ? "live-status--warn" : "live-status--ok"}`}
                      >
                        {status}
                      </span>
                    </td>
                    <td data-label="Revision" className="live-num">
                      {lab.revision}
                    </td>
                    {isTa ? (
                      <td className="live-cell-action">
                        <Link
                          className="live-button live-button-secondary live-button-sm"
                          href={`/live/labs/${lab.id}/reviews`}
                        >
                          Review
                        </Link>
                      </td>
                    ) : null}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
