import React from "react";

import { formatDateTime } from "../../lib/live/format";
import { projectStudentStatus } from "../../lib/live/view-rules";
import type { SubmissionState } from "../../lib/live/view-rules";

const STEPS: readonly SubmissionState[] = ["queued", "judging", "judged"];
const STEP_LABELS: Record<SubmissionState, string> = {
  queued: "Queued",
  judging: "Judging",
  judged: "Judged",
};

void React;

export function StudentSubmissionStatus({ payload }: { readonly payload: unknown }) {
  try {
    const status = projectStudentStatus(payload);
    const current = STEPS.indexOf(status.state);
    return (
      <article className="live-page" aria-labelledby="submission-status-heading">
        <header className="live-page-head">
          <div className="live-heading-group">
            <h1 id="submission-status-heading">Submission status</h1>
            <p className="live-muted">
              Received <time dateTime={status.receivedAt}>{formatDateTime(status.receivedAt)}</time>
            </p>
          </div>
        </header>
        <ol className="live-steps" aria-label="Judge progress">
          {STEPS.map((step, index) => (
            <li
              key={step}
              data-step={
                index < current || status.state === "judged"
                  ? "done"
                  : index === current
                    ? "current"
                    : "todo"
              }
            >
              {STEP_LABELS[step]}
            </li>
          ))}
        </ol>
        <dl className="live-facts">
          <div>
            <dt>State</dt>
            <dd>{status.state}</dd>
          </div>
          <div>
            <dt>Judge</dt>
            <dd>
              {status.judge ? (
                <span
                  className={`live-status ${status.judge.verdict === "AC" ? "live-status--ok" : "live-status--bad"}`}
                >
                  {status.judge.verdict}
                </span>
              ) : (
                "Not judged yet"
              )}
            </dd>
          </div>
        </dl>
        {status.judge ? (
          <div className="live-panel live-table-wrap">
            <table className="live-table">
              <caption>Test timing</caption>
              <thead>
                <tr>
                  <th scope="col">Test</th>
                  <th scope="col">Judge result</th>
                  <th scope="col" className="live-num">
                    Time (ms)
                  </th>
                </tr>
              </thead>
              <tbody>
                {status.judge.tests.map((test) => (
                  <tr key={test.id}>
                    <td>
                      <code>{test.id}</code>
                    </td>
                    <td>
                      <span
                        className={`live-status ${test.verdict === "AC" ? "live-status--ok" : "live-status--bad"}`}
                      >
                        {test.verdict}
                      </span>
                    </td>
                    <td className="live-num">{test.timeMs}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </article>
    );
  } catch {
    return (
      <p className="live-alert" role="alert">
        This result cannot be shown.
      </p>
    );
  }
}
