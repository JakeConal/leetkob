"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";

import { getCurrentUser, readLab, submitSource } from "../../lib/live/api";
import { projectStudentLab } from "../../lib/live/view-rules";
import type { StudentLabView } from "../../lib/live/view-rules";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
import { formatDateTime } from "../../lib/live/format";
import { useLiveSession } from "./live-shell";
import { Icon } from "../icon";
import { SubmissionScreen } from "./submission-screen";

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
  return <LabBody key={lab.data.id} lab={lab.data} />;
}

function LabBody({ lab }: { readonly lab: StudentLabView }) {
  const session = useLiveSession();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const [source, setSource] = useState("");
  const [activeTab, setActiveTab] = useState("Description");
  const [submissionId, setSubmissionId] = useState<string | null>(null);
  const gutterRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    setSource("");
    setSubmissionId(null);
  }, [me.data?.user.id]);
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
    if (pending || me.data?.user.role !== "student") return;
    if (new TextEncoder().encode(source).length > 65536) {
      setFormError(new Error("The source must be at most 65,536 UTF-8 bytes."));
      return;
    }
    setPending(true);
    try {
      const accepted = await submitSource(lab.id, source, session.csrfToken, crypto.randomUUID());
      setSubmissionId(accepted.id);
    } catch (error) {
      setFormError(error);
    } finally {
      setPending(false);
    }
  }

  const isTa = me.data?.user.role === "ta";
  return (
    <div className="practice-lab">
      <h1 className="sr-only">Lab workspace</h1>
      <div className="lab-workspace-toolbar">
        <div>
          <Link href="/">
            <Icon name="labs" size={17} />
            Your labs
          </Link>
          <Icon name="chevron" size={14} />
          <span>Lab workspace</span>
        </div>
        {isTa ? (
          <Link className="secondary-action" href={`/live/labs/${lab.id}/reviews`}>
            Open review queue
            <Icon name="arrow" size={16} />
          </Link>
        ) : (
          <span className="workspace-language">C++17</span>
        )}
      </div>
      <div className="coding-workspace">
        <div className="problem-pane">
          <div className="problem-tabs" aria-label="Problem sections">
            {["Description", "Constraints", "Sample tests"].map((tab) => (
              <button
                type="button"
                key={tab}
                aria-pressed={activeTab === tab}
                onClick={() => setActiveTab(tab)}
              >
                {tab}
              </button>
            ))}
          </div>
          <section
            className="panel"
            hidden={activeTab !== "Description"}
            aria-labelledby="lab-heading"
          >
            <div className="panel-heading">
              <p className="panel-label">Problem</p>
              <h2 id="lab-heading">{lab.title}</h2>
              <p>Revision {lab.revision} · C++17</p>
            </div>
            <dl className="lab-facts">
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
            <div className="prose-block">
              <h3>Statement</h3>
              <p className="statement">{lab.statement}</p>
            </div>
          </section>
          <section
            className="panel model-notice"
            hidden={activeTab !== "Description"}
            aria-labelledby="model-heading"
          >
            <p className="panel-label panel-label--accent">Model notice</p>
            <h2 id="model-heading">Model-assisted. TA-reviewed.</h2>
            <p>{lab.modelNotice}</p>
            <p>Model output never publishes a deduction or a mark.</p>
          </section>
          <section
            className="panel"
            hidden={activeTab !== "Constraints"}
            aria-labelledby="constraints-heading"
          >
            <div className="panel-heading">
              <h2 id="constraints-heading">Constraints & deductions</h2>
              <p>
                Deductions are configured by your TA. Check these rules before writing your
                solution.
              </p>
            </div>
            <ul className="live-constraints">
              {lab.constraints.map((constraint) => (
                <li key={constraint.id}>
                  <code>{constraint.id}</code>
                  <p>{constraint.text}</p>
                  <span className="live-num">−{constraint.deduction}</span>
                </li>
              ))}
            </ul>
          </section>
          <section
            className="panel"
            hidden={activeTab !== "Sample tests"}
            aria-labelledby="samples-heading"
          >
            <div className="panel-heading">
              <h2 id="samples-heading">Sample tests</h2>
              <p>Hidden inputs and expected outputs stay on the server.</p>
            </div>
            {lab.sampleTests.length === 0 ? (
              <p>No sample tests were published.</p>
            ) : (
              lab.sampleTests.map((test) => (
                <article className="sample-test" key={test.id}>
                  <h3>Test {test.id}</h3>
                  <p className="panel-label">Input</p>
                  <pre className="code-block" tabIndex={0} aria-label={`${test.id} input`}>
                    {test.input}
                  </pre>
                  <p className="panel-label">Expected output</p>
                  <pre
                    className="code-block"
                    tabIndex={0}
                    aria-label={`${test.id} expected output`}
                  >
                    {test.expectedOutput}
                  </pre>
                </article>
              ))
            )}
          </section>
        </div>
        <div className="solution-pane">
          <section className="submission-editor" aria-labelledby="code-heading">
            <div className="editor-heading">
              <h2 id="code-heading">
                <Icon name="code" size={18} />
                Code
              </h2>
              <span>C++17</span>
              <button
                type="button"
                onClick={() => setSource("")}
                disabled={pending || source === ""}
              >
                Clear
              </button>
            </div>
            <form className="editor-form" onSubmit={(event) => void onSubmit(event)}>
              <label className="sr-only" htmlFor="submission-source">
                C++17 source
              </label>
              <div className="code-editor">
                <div className="line-numbers" ref={gutterRef} aria-hidden="true">
                  {Array.from({ length: Math.max(20, source.split("\n").length) }, (_, index) => (
                    <span key={index}>{index + 1}</span>
                  ))}
                </div>
                <textarea
                  id="submission-source"
                  spellCheck={false}
                  autoCapitalize="off"
                  autoCorrect="off"
                  required
                  value={source}
                  onChange={(event) => setSource(event.target.value)}
                  onScroll={(event) => {
                    if (gutterRef.current !== null)
                      gutterRef.current.scrollTop = event.currentTarget.scrollTop;
                  }}
                  placeholder={
                    "// Write your C++17 solution here.\n// Read the statement and constraints before submitting.\n\n#include <iostream>\n\nint main() {\n    // Your solution\n    return 0;\n}"
                  }
                  aria-describedby="submission-source-hint"
                />
              </div>
              <div className="editor-status">
                <span>UTF-8</span>
                <span>
                  {new TextEncoder().encode(source).length.toLocaleString("en-GB")} / 65,536 bytes
                </span>
              </div>
              {formError ? <LiveError error={formError} /> : null}
              <div className="editor-submit-bar">
                <p className="field-hint" id="submission-source-hint">
                  Submit before the lab deadline.
                  <br />
                  You can submit again while the lab is open.
                </p>
                <button
                  className="primary-action"
                  type="submit"
                  disabled={
                    pending || session.csrfToken === null || me.data?.user.role !== "student"
                  }
                >
                  {pending ? "Submitting…" : "Submit for judging"}
                  <Icon name="arrow" size={16} />
                </button>
              </div>
              {me.data?.user.role !== "student" ? (
                <p className="editor-notice">Only an enrolled student can submit a solution.</p>
              ) : null}
            </form>
            <div className="result-heading">
              <Icon name="review" size={17} />
              <h3>Submission result</h3>
            </div>
            {submissionId === null ? (
              <div className="result-empty">
                <Icon name="code" size={25} />
                <p>Your results will appear here.</p>
                <small>Submit a solution to see test verdicts and timings.</small>
              </div>
            ) : (
              <div className="submission-result" aria-live="polite">
                <SubmissionScreen submissionId={submissionId} />
                <Link className="inline-action" href={`/live/submissions/${submissionId}`}>
                  Open submission details
                </Link>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
