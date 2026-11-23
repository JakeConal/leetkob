"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { FormEvent } from "react";

import { getCurrentUser, readLab, submitSource } from "../../lib/live/api";
import { projectStudentLab } from "../../lib/live/view-rules";
import type { StudentLabView } from "../../lib/live/view-rules";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
import { useLiveSession } from "./live-shell";

const queryOptions = { retry: false, gcTime: 0, staleTime: 0 } as const;

export function StudentLab({ labId }: { readonly labId: string }) {
  const session = useLiveSession();
  const lab = useQuery({
    queryKey: ["live", "lab", labId],
    queryFn: async () => projectStudentLab(await readLab(labId)),
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const notFound = useReplaceNotFound(lab.error);
  if (!session.ready || session.closing) return <p className="live-muted">Loading…</p>;
  if (notFound) return <LiveNotFound />;
  if (lab.isPending) return <p className="live-muted">Loading lab…</p>;
  if (lab.isError) return <LiveError error={lab.error} />;
  if (!lab.data) return null;
  return <LabBody lab={lab.data} />;
}

function LabBody({ lab }: { readonly lab: StudentLabView }) {
  const session = useLiveSession();
  const router = useRouter();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const [source, setSource] = useState("");
  const [formError, setFormError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    if (session.csrfToken === null) {
      setFormError(new Error("Sign in again before submitting."));
      return;
    }
    if (source.trim() === "") {
      setFormError(new Error("Enter the C++ source to submit."));
      return;
    }
    setPending(true);
    try {
      const accepted = await submitSource(lab.id, source, session.csrfToken, crypto.randomUUID());
      router.push(`/live/submissions/${accepted.id}`);
    } catch (error) {
      setFormError(error);
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="live-stack">
      <article className="live-card">
        <h1>{lab.title}</h1>
        <p className="live-muted">Revision {lab.revision}</p>
        <aside className="live-notice" aria-label="Model review notice">
          <h2>Model review</h2>
          <p>{lab.modelNotice}</p>
        </aside>
        <h2>Statement</h2>
        <p className="live-statement">{lab.statement}</p>
        <h2>Schedule and limits</h2>
        <dl className="live-meta">
          <div>
            <dt>Opens</dt>
            <dd>
              <time dateTime={lab.opensAt}>{lab.opensAt}</time>
            </dd>
          </div>
          <div>
            <dt>Deadline</dt>
            <dd>
              <time dateTime={lab.deadline}>{lab.deadline}</time>
            </dd>
          </div>
          <div>
            <dt>CPU seconds</dt>
            <dd>{lab.limits.cpuSeconds}</dd>
          </div>
          <div>
            <dt>Memory MiB</dt>
            <dd>{lab.limits.memoryMiB}</dd>
          </div>
        </dl>
        <h2>Constraints</h2>
        <ul className="live-list">
          {lab.constraints.map((constraint) => (
            <li key={constraint.id}>
              <strong>{constraint.id}</strong> deduction {constraint.deduction}
              <p>{constraint.text}</p>
            </li>
          ))}
        </ul>
        <h2>Sample tests</h2>
        {lab.sampleTests.length === 0 ? <p>No sample tests were published.</p> : null}
        {lab.sampleTests.map((test) => (
          <section key={test.id}>
            <h3>{test.id}</h3>
            <pre className="live-source" tabIndex={0} aria-label={`${test.id} input`}>
              {test.input}
            </pre>
            <pre className="live-source" tabIndex={0} aria-label={`${test.id} expected output`}>
              {test.expectedOutput}
            </pre>
          </section>
        ))}
        <p>Hidden test contents stay on the server.</p>
        {me.data?.user.role === "ta" ? (
          <Link className="live-button" href={`/live/labs/${lab.id}/reviews`}>
            Open review
          </Link>
        ) : null}
      </article>
      <form className="live-card live-form" onSubmit={(event) => void onSubmit(event)}>
        <h2>Submit source</h2>
        {formError ? <LiveError error={formError} /> : null}
        <label className="live-field" htmlFor="submission-source">
          C++17 source
          <textarea
            id="submission-source"
            rows={12}
            spellCheck={false}
            value={source}
            onChange={(event) => setSource(event.target.value)}
          />
        </label>
        <button className="live-button" type="submit" disabled={pending}>
          {pending ? "Submitting…" : "Submit"}
        </button>
      </form>
    </div>
  );
}
