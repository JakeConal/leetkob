"use client";

import Link from "next/link";
import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent } from "react";

import {
  ApiError,
  createIdempotencyKey,
  summarizeError,
  type ApiErrorSummary,
} from "../../../lib/api-client";
import { getSession, endSession, subscribeProtectedState } from "../../../lib/protected-state";
import { formatTimestamp } from "../../../lib/format";
import {
  pollSubmissionUntilJudged,
  submitSource,
  type Submission,
} from "../../../lib/student-submission";

const MAX_SOURCE_BYTES = 65536;

type PanelState =
  | { readonly status: "idle" }
  | { readonly status: "submitting" }
  | { readonly status: "polling"; readonly submission: Submission }
  | { readonly status: "judged"; readonly submission: Submission }
  | { readonly status: "error"; readonly problem: ApiErrorSummary };

interface PendingKey {
  readonly key: string;
  readonly source: string;
}

export function SubmissionPanel({ labId }: Readonly<{ labId: string }>) {
  const session = useSyncExternalStore(subscribeProtectedState, getSession, () => null);
  const [source, setSource] = useState("");
  const [state, setState] = useState<PanelState>({ status: "idle" });
  const pendingKey = useRef<PendingKey | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  const busy = state.status === "submitting" || state.status === "polling";

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (session === null || busy) return;

    const bytes = new TextEncoder().encode(source).length;
    if (bytes === 0 || bytes > MAX_SOURCE_BYTES) {
      setState({
        status: "error",
        problem: {
          title: "Check the source",
          detail:
            bytes === 0
              ? "Enter the C++17 source to submit."
              : `The source is ${bytes} bytes; the maximum is ${MAX_SOURCE_BYTES}.`,
          status: null,
          type: null,
          instance: null,
          retryAfterSeconds: null,
        },
      });
      return;
    }

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const previous = pendingKey.current;
    const key =
      previous !== null && previous.source === source ? previous.key : createIdempotencyKey();
    pendingKey.current = { key, source };
    setState({ status: "submitting" });

    try {
      const accepted = await submitSource(labId, source, {
        csrfToken: session.csrfToken,
        idempotencyKey: key,
        signal: controller.signal,
      });
      pendingKey.current = null;
      const queued: Submission = {
        id: accepted.id,
        labId: accepted.labId,
        state: accepted.state,
        receivedAt: accepted.receivedAt,
      };
      setState({ status: "polling", submission: queued });
      const judged = await pollSubmissionUntilJudged({
        submissionId: accepted.id,
        signal: controller.signal,
        intervalMs: 1000,
        maxAttempts: 1800,
        onUpdate: (submission) => {
          if (controller.signal.aborted) return;
          setState({
            status: submission.state === "judged" ? "judged" : "polling",
            submission,
          });
        },
      });
      if (!controller.signal.aborted) {
        setState({ status: "judged", submission: judged });
      }
    } catch (error) {
      if (controller.signal.aborted) return;
      if (error instanceof ApiError && error.status === 401) {
        endSession("authentication-required");
        return;
      }
      setState({ status: "error", problem: summarizeError(error) });
    }
  }

  return (
    <section className="panel" aria-labelledby="submission-heading">
      <div className="panel-heading">
        <p className="panel-label">Your submission</p>
        <h2 id="submission-heading">Submit C++17</h2>
        <p>
          The judge compiles once at -O2 and compares output tokens per test. You see only your own
          per-test verdicts and timings; hidden inputs, expected outputs and checker findings stay
          on the server.
        </p>
      </div>

      {session === null ? (
        <div className="actions">
          <p>The CSRF proof is kept in memory only, so submitting needs a fresh sign-in.</p>
          <Link className="primary-action" href="/sign-in">
            Sign in
          </Link>
        </div>
      ) : session.user.role !== "student" ? (
        <p role="status">
          Signed in as {session.user.displayName} ({session.user.role}). Only an enrolled student
          can submit; the API enforces this even if the form is reached directly.
        </p>
      ) : (
        <form className="form-stack" onSubmit={(event) => void handleSubmit(event)} noValidate>
          <div className="field">
            <label htmlFor="submission-source">Source</label>
            <textarea
              id="submission-source"
              name="source"
              rows={12}
              required
              spellCheck={false}
              value={source}
              onChange={(event) => setSource(event.target.value)}
              aria-describedby="submission-source-hint"
            />
            <p className="field-hint" id="submission-source-hint">
              At most {MAX_SOURCE_BYTES.toLocaleString("en-GB")} UTF-8 bytes. The submission is
              accepted only while the lab is open and before its deadline.
            </p>
          </div>

          {state.status === "error" ? (
            <div className="problem-alert" role="alert">
              <p className="problem-alert-title">{state.problem.title}</p>
              <p>{state.problem.detail}</p>
              <p className="problem-meta">
                {state.problem.status === null ? null : <span>HTTP {state.problem.status}</span>}
                {state.problem.type === null ? null : <span>{state.problem.type}</span>}
              </p>
            </div>
          ) : null}

          <div className="actions">
            <button className="primary-action" type="submit" disabled={busy}>
              {state.status === "submitting" ? "Submitting…" : "Submit for judging"}
            </button>
          </div>
        </form>
      )}

      {state.status === "polling" ? (
        <div className="prose-block" role="status" aria-live="polite">
          <h3>Waiting for the judge</h3>
          <p>
            Submission {state.submission.id} is <strong>{state.submission.state}</strong>. This
            screen refetches the current status until it is judged.
          </p>
        </div>
      ) : null}

      {state.status === "judged" ? <JudgedResult submission={state.submission} /> : null}
    </section>
  );
}

function JudgedResult({ submission }: Readonly<{ submission: Submission }>) {
  const judge = submission.judge;
  if (judge === undefined) return null;
  return (
    <div className="prose-block" aria-labelledby="judge-result-heading">
      <p className="panel-label panel-label--accent">Judged</p>
      <h3 id="judge-result-heading">Verdict: {judge.verdict}</h3>
      <p>
        Received {formatTimestamp(submission.receivedAt)}. Per-test verdicts and timings only;
        hidden test data is never included.
      </p>
      <div
        className="table-region"
        role="region"
        aria-labelledby="judge-result-heading"
        tabIndex={0}
      >
        <table className="data-table">
          <thead>
            <tr>
              <th scope="col">Test</th>
              <th scope="col">Verdict</th>
              <th scope="col">Time (ms)</th>
            </tr>
          </thead>
          <tbody>
            {judge.tests.map((test) => (
              <tr key={test.id}>
                <td>{test.id}</td>
                <td>{test.verdict}</td>
                <td>{test.timeMs}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {judge.tests.length === 0 ? <p>Compilation failed, so no test executed.</p> : null}
    </div>
  );
}
