import React from "react";

import { projectStudentStatus } from "../../lib/live/view-rules";

void React;

export function StudentSubmissionStatus({ payload }: { readonly payload: unknown }) {
  try {
    const status = projectStudentStatus(payload);
    return (
      <article className="live-card" aria-labelledby="submission-status-heading">
        <h1 id="submission-status-heading">Submission status</h1>
        <dl className="live-meta">
          <div>
            <dt>State</dt>
            <dd>{status.state}</dd>
          </div>
          <div>
            <dt>Received</dt>
            <dd>
              <time dateTime={status.receivedAt}>{status.receivedAt}</time>
            </dd>
          </div>
          <div>
            <dt>Judge</dt>
            <dd>{status.judge ? status.judge.verdict : "Not judged yet"}</dd>
          </div>
        </dl>
        {status.judge ? (
          <table className="live-table">
            <caption>Test timing</caption>
            <thead>
              <tr>
                <th scope="col">Test</th>
                <th scope="col">Judge result</th>
                <th scope="col">Time (ms)</th>
              </tr>
            </thead>
            <tbody>
              {status.judge.tests.map((test) => (
                <tr key={test.id}>
                  <td>{test.id}</td>
                  <td>{test.verdict}</td>
                  <td>{test.timeMs}</td>
                </tr>
              ))}
            </tbody>
          </table>
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
