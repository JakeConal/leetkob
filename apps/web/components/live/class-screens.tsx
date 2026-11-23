"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";

import { ApiProblem, getCurrentUser, listClasses, listLabs } from "../../lib/live/api";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
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

  if (!session.ready || session.closing) return <p className="live-muted">Loading…</p>;
  if (signedOut || notFound)
    return notFound ? <LiveNotFound /> : <p className="live-muted">Sign in required.</p>;
  if (me.isPending || classes.isPending) return <p className="live-muted">Loading classes…</p>;
  if (me.isError) return <LiveError error={me.error} />;
  if (classes.isError) return <LiveError error={classes.error} />;
  if (!classes.data) return null;

  return (
    <section className="live-card" aria-labelledby="classes-heading">
      <h1 id="classes-heading">Classes</h1>
      <p className="live-muted">
        {me.data.user.displayName} · {me.data.user.role}
      </p>
      {classes.data.items.length === 0 ? (
        <p>No classes are assigned to this account.</p>
      ) : (
        <ul className="live-list">
          {classes.data.items.map((item) => (
            <li key={item.id}>
              <Link href={`/live/classes/${item.id}`}>{item.name}</Link>
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

  if (!session.ready || session.closing) return <p className="live-muted">Loading…</p>;
  if (signedOut) return <p className="live-muted">Sign in required.</p>;
  if (notFound) return <LiveNotFound />;
  if (me.isPending || labs.isPending) return <p className="live-muted">Loading labs…</p>;
  if (me.isError) return <LiveError error={me.error} />;
  if (labs.isError) return <LiveError error={labs.error} />;
  if (!labs.data || !me.data) return null;

  const isTa = me.data.user.role === "ta";
  return (
    <section className="live-card" aria-labelledby="labs-heading">
      <h1 id="labs-heading">Labs</h1>
      {isTa ? (
        <Link className="live-button" href={`/live/classes/${classId}/labs/new`}>
          Create lab
        </Link>
      ) : null}
      {labs.data.items.length === 0 ? (
        <p>No labs are visible in this class.</p>
      ) : (
        <ul className="live-list">
          {labs.data.items.map((lab) => (
            <li key={lab.id}>
              <Link href={`/live/labs/${lab.id}`}>{lab.title}</Link>
              <span className="live-muted">Revision {lab.revision}</span>
              {isTa ? <Link href={`/live/labs/${lab.id}/reviews`}>Review</Link> : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
