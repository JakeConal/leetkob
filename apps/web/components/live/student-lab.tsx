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
import { formatDateTime } from "../../lib/live/format";
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
  if (!session.ready || session.closing) return <p className="live-loading">Loading…</p>;
  if (notFound) return <LiveNotFound />;
  if (lab.isPending) return <p className="live-loading">Loading lab…</p>;
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

  const isTa = me.data?.user.role === "ta";
  return (
    <div className="live-page">
      <nav className="live-crumbs" aria-label="Breadcrumb">
        <Link href="/live/classes">Classes</Link>
        <span aria-hidden="true">/</span>
        <Link href={`/live/classes/${lab.classId}`}>Labs</Link>
        <span aria-hidden="true">/</span>
        <span aria-current="page">{lab.title}</span>
      </nav>
      <header className="live-page-head">
        <div className="live-heading-group">
          <h1>{lab.title}</h1>
          <p className="live-muted">Revision {lab.revision}</p>
        </div>
        {isTa ? (
          <Link className="live-button live-button-secondary" href={`/live/labs/${lab.id}/reviews`}>
            Open review
          </Link>
        ) : null}
      </header>
      <dl className="live-facts">
        <div>
          <dt>Opens</dt>
          <dd>
            <time dateTime={lab.opensAt}>{formatDateTime(lab.opensAt)}</time>
          </dd>
        </div>
        <div>
          <dt>Deadline</dt>
          <dd>
            <time dateTime={lab.deadline}>{formatDateTime(lab.deadline)}</time>
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
      <div className="live-lab-layout">
        <div className="live-prose-column">
          <section aria-labelledby="statement-heading" className="live-section">
            <h2 id="statement-heading">Statement</h2>
            <p className="live-statement">{lab.statement}</p>
          </section>
          <section aria-labelledby="constraints-heading" className="live-section">
            <h2 id="constraints-heading">Constraints</h2>
            <ul className="live-constraints">
              {lab.constraints.map((constraint) => (
                <li key={constraint.id}>
                  <code>{constraint.id}</code>
                  <p>{constraint.text}</p>
                  <span className="live-num">−{constraint.deduction}</span>
                </li>
              ))}
            </ul>
            <aside className="live-note" aria-label="Model review notice">
              <h2>Model review</h2>
              <p>{lab.modelNotice}</p>
            </aside>
          </section>
          <section aria-labelledby="samples-heading" className="live-section">
            <h2 id="samples-heading">Sample tests</h2>
            {lab.sampleTests.length === 0 ? (
              <p className="live-muted">No sample tests were published.</p>
            ) : null}
            {lab.sampleTests.map((test) => (
              <div key={test.id} className="live-sample">
                <h3>{test.id}</h3>
                <div className="live-sample-io">
                  <div>
                    <span className="live-label">Input</span>
                    <pre className="live-code" tabIndex={0} aria-label={`${test.id} input`}>
                      {test.input}
                    </pre>
                  </div>
                  <div>
                    <span className="live-label">Expected output</span>
                    <pre
                      className="live-code"
                      tabIndex={0}
                      aria-label={`${test.id} expected output`}
                    >
                      {test.expectedOutput}
                    </pre>
                  </div>
                </div>
              </div>
            ))}
            <p className="live-muted">Hidden test contents stay on the server.</p>
          </section>
        </div>
        <form
          className="live-panel live-form live-submit"
          onSubmit={(event) => void onSubmit(event)}
        >
          <div className="live-heading-group">
            <h2>Submit source</h2>
            <p className="live-muted">Compiled and judged in an isolated sandbox.</p>
          </div>
          {formError ? <LiveError error={formError} /> : null}
          <label className="live-field" htmlFor="submission-source">
            C++17 source
            <textarea
              id="submission-source"
              className="live-editor"
              rows={18}
              spellCheck={false}
              value={source}
              onChange={(event) => setSource(event.target.value)}
            />
          </label>
          <button className="live-button live-button-block" type="submit" disabled={pending}>
            {pending ? "Submitting…" : "Submit"}
          </button>
        </form>
      </div>
    </div>
  );
}
