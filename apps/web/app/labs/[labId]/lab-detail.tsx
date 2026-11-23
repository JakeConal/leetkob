"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { ApiError, summarizeError, type ApiErrorSummary } from "../../../lib/api-client";
import type { LabDetail } from "../../../lib/api-types";
import { formatTimestamp } from "../../../lib/format";
import { fetchLab } from "../../../lib/session-client";

type ViewState =
  | { readonly status: "loading" }
  | { readonly status: "ready"; readonly lab: LabDetail }
  | {
      readonly status: "error";
      readonly problem: ApiErrorSummary;
      readonly unauthenticated: boolean;
    };

const FALLBACK_MODEL_NOTICE =
  "Semantic constraints are reviewed by a model; a TA approves any deduction before release.";

export function LabDetailPanel({ labId }: Readonly<{ labId: string }>) {
  const [view, setView] = useState<ViewState>({ status: "loading" });

  useEffect(() => {
    let active = true;
    setView({ status: "loading" });
    void (async () => {
      try {
        const lab = await fetchLab(labId);
        if (active) setView({ status: "ready", lab });
      } catch (error) {
        if (!active) return;
        const unauthenticated = error instanceof ApiError && error.status === 401;
        setView({ status: "error", problem: summarizeError(error), unauthenticated });
      }
    })();
    return () => {
      active = false;
    };
  }, [labId]);

  if (view.status === "loading") {
    return (
      <section className="panel" aria-labelledby="loading-heading">
        <div className="panel-heading">
          <h2 id="loading-heading">Loading lab</h2>
          <p role="status">Reading the safe lab projection…</p>
        </div>
      </section>
    );
  }

  if (view.status === "error") {
    return (
      <section className="panel" aria-labelledby="error-heading">
        <div className="panel-heading">
          <h2 id="error-heading">Lab unavailable</h2>
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
          {view.unauthenticated ? (
            <Link className="primary-action" href="/sign-in">
              Sign in
            </Link>
          ) : null}
          <Link className="inline-action" href="/classes">
            Back to classes
          </Link>
        </div>
      </section>
    );
  }

  const { lab } = view;

  return (
    <>
      <section className="panel" aria-labelledby="lab-heading">
        <div className="panel-heading">
          <p className="panel-label">Class lab</p>
          <h2 id="lab-heading">{lab.title}</h2>
          <p>
            Latest revision {lab.revision}. Inputs are immutable in this checkpoint; lab revision
            endpoints are not registered.
          </p>
        </div>

        <dl className="lab-facts">
          <div>
            <dt>Class</dt>
            <dd>{lab.classId}</dd>
          </div>
          <div>
            <dt>Opens</dt>
            <dd>{formatTimestamp(lab.opensAt)}</dd>
          </div>
          <div>
            <dt>Deadline</dt>
            <dd>{formatTimestamp(lab.deadline)}</dd>
          </div>
          <div>
            <dt>Limits</dt>
            <dd>
              {lab.limits.cpuSeconds} s CPU · {lab.limits.memoryMiB} MiB
            </dd>
          </div>
        </dl>

        <div className="prose-block">
          <h3>Statement</h3>
          <p className="statement">{lab.statement}</p>
        </div>
      </section>

      <section className="panel model-notice" aria-labelledby="model-notice-heading">
        <p className="panel-label panel-label--accent">Model notice</p>
        <h2 id="model-notice-heading">Semantic review is model-assisted, TA-approved</h2>
        <p>{lab.modelNotice === "" ? FALLBACK_MODEL_NOTICE : lab.modelNotice}</p>
        <p className="field-hint">
          Model output is per-constraint pass, violation or unsure evidence only. It never publishes
          a deduction or a mark.
        </p>
      </section>

      <section className="panel" aria-labelledby="constraints-heading">
        <div className="panel-heading">
          <p className="panel-label">Constraints</p>
          <h2 id="constraints-heading">Deductions</h2>
          <p>Deductions are TA-configured; the checker never chooses one.</p>
        </div>
        <div
          className="table-region"
          role="region"
          aria-labelledby="constraints-heading"
          tabIndex={0}
        >
          <table className="data-table">
            <thead>
              <tr>
                <th scope="col">ID</th>
                <th scope="col">Constraint</th>
                <th scope="col">Deduction</th>
              </tr>
            </thead>
            <tbody>
              {lab.constraints.map((constraint) => (
                <tr key={constraint.id}>
                  <td>{constraint.id}</td>
                  <td>{constraint.text}</td>
                  <td>{constraint.deduction}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="panel" aria-labelledby="samples-heading">
        <div className="panel-heading">
          <p className="panel-label">Visible tests</p>
          <h2 id="samples-heading">Sample tests</h2>
          <p>
            {lab.sampleTests.length === 0
              ? "This lab has no sample tests."
              : `${lab.sampleTests.length} sample ${lab.sampleTests.length === 1 ? "test" : "tests"}. Hidden tests stay on the server.`}
          </p>
        </div>

        <div className="sample-list">
          {lab.sampleTests.map((test, index) => (
            <article className="sample-test" key={test.id} aria-labelledby={`sample-${index}`}>
              <h3 id={`sample-${index}`}>Test {test.id}</h3>
              <p className="panel-label">Input</p>
              <pre className="code-block">{test.input}</pre>
              <p className="panel-label">Expected output</p>
              <pre className="code-block">{test.expectedOutput}</pre>
            </article>
          ))}
        </div>
      </section>
    </>
  );
}
