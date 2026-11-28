"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { FormEvent } from "react";

import {
  deadlineHasPassed,
  decideFinding,
  expectedFindingRevision,
  listReviews,
  readLab,
} from "../../lib/live/api";
import type { Evidence, Finding } from "../../lib/live/api";
import { projectStudentLab } from "../../lib/live/view-rules";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
import { useLiveSession } from "./live-shell";
import { PublishControl } from "./publish-control";
import { SavedTaDecision } from "./saved-ta-decision";

const queryOptions = { retry: false, gcTime: 0, staleTime: 0 } as const;

export function ReviewQueue({ labId }: { readonly labId: string }) {
  const session = useLiveSession();
  const reviews = useQuery({
    queryKey: ["live", "reviews", labId],
    queryFn: () => listReviews(labId, null),
    enabled: session.ready && !session.closing,
    ...queryOptions,
  });
  const lab = useQuery({
    queryKey: ["live", "lab", labId],
    queryFn: async () => projectStudentLab(await readLab(labId)),
    enabled: session.ready && !session.closing && reviews.isSuccess,
    ...queryOptions,
  });
  const notFound = useReplaceNotFound(reviews.error ?? lab.error);
  const [extra, setExtra] = useState<readonly Finding[]>([]);
  const [cursor, setCursor] = useState<string | null | undefined>(undefined);
  const [paging, setPaging] = useState(false);
  const [pageError, setPageError] = useState<unknown>(null);

  if (!session.ready || session.closing) return <p className="live-loading">Loading…</p>;
  if (notFound) return <LiveNotFound />;
  if (reviews.isPending) {
    return (
      <div className="live-page">
        <PublishControl loading />
        <p className="live-loading">Loading review queue…</p>
      </div>
    );
  }
  if (reviews.isError) return <LiveError error={reviews.error} />;
  if (!reviews.data) return null;

  const items = [...reviews.data.items, ...extra];
  const nextCursor = cursor === undefined ? reviews.data.nextCursor : cursor;
  const deadlinePassed = lab.data ? deadlineHasPassed(lab.data.deadline, Date.now()) : null;

  async function loadMore() {
    if (nextCursor === null) return;
    setPaging(true);
    setPageError(null);
    try {
      const page = await listReviews(labId, nextCursor);
      setExtra((current) => [...current, ...page.items]);
      setCursor(page.nextCursor);
    } catch (error) {
      setPageError(error);
    } finally {
      setPaging(false);
    }
  }

  const groups = new Map<string, Finding[]>();
  for (const finding of items) {
    const group = groups.get(finding.submissionId) ?? [];
    group.push(finding);
    groups.set(finding.submissionId, group);
  }

  return (
    <div className="live-page">
      {lab.data ? (
        <nav className="live-crumbs" aria-label="Breadcrumb">
          <a href="/live/classes">Classes</a>
          <span aria-hidden="true">/</span>
          <a href={`/live/labs/${labId}`}>{lab.data.title}</a>
          <span aria-hidden="true">/</span>
          <span aria-current="page">Review</span>
        </nav>
      ) : null}
      <header className="live-page-head">
        <div className="live-heading-group">
          <h1>Review queue</h1>
          <p className="live-muted">
            {groups.size} {groups.size === 1 ? "submission" : "submissions"} · {items.length}{" "}
            {items.length === 1 ? "finding" : "findings"} loaded
          </p>
        </div>
      </header>
      <PublishControl
        reviewPending={reviews.data.reviewPending}
        processingPending={reviews.data.processingPending}
        deadlinePassed={deadlinePassed}
      />
      {items.length === 0 ? <p className="live-empty">No findings are in this queue.</p> : null}
      {[...groups.entries()].map(([submissionId, findings]) => (
        <section
          key={submissionId}
          className="live-submission-group"
          aria-label={`Submission ${submissionId}`}
        >
          <h2 className="live-group-title">
            Submission <code>{submissionId}</code>
            <span className="live-muted">
              {findings.length} {findings.length === 1 ? "constraint" : "constraints"}
            </span>
          </h2>
          {findings.map((finding) => (
            <FindingReview key={finding.id} labId={labId} finding={finding} />
          ))}
        </section>
      ))}
      {pageError ? <LiveError error={pageError} /> : null}
      {nextCursor !== null ? (
        <button
          className="live-button live-button-secondary"
          type="button"
          disabled={paging}
          onClick={() => void loadMore()}
        >
          {paging ? "Loading…" : "Load more"}
        </button>
      ) : null}
    </div>
  );
}

function FindingReview({ labId, finding }: { readonly labId: string; readonly finding: Finding }) {
  const session = useLiveSession();
  const queryClient = useQueryClient();
  const [choice, setChoice] = useState<"pass" | "violation" | "unsure" | null>(null);
  const [reason, setReason] = useState("");
  const [evidenceKey, setEvidenceKey] = useState<string | null>(null);
  const [message, setMessage] = useState<unknown>(null);
  const [pending, setPending] = useState(false);

  const configured = finding.deduction;
  const chosen =
    choice === null
      ? finding.decision
        ? "No new decision selected."
        : "No deduction chosen yet."
      : choice === "pass"
        ? "Chosen deduction: 0. The configured deduction is not applied."
        : choice === "violation"
          ? `Chosen deduction: ${configured}.`
          : "Chosen deduction: unsure. No deduction is confirmed.";

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage(null);
    if (choice === null) {
      setMessage(new Error("Choose pass, violation, or unsure."));
      return;
    }
    if (choice === "unsure") {
      setMessage(
        new Error("Unsure is explicit. No deduction is confirmed, and no decision was sent."),
      );
      return;
    }
    if (reason.trim() === "") {
      setMessage(new Error("Enter a reason."));
      return;
    }
    if (session.csrfToken === null) {
      setMessage(new Error("Sign in again before recording a decision."));
      return;
    }
    const evidence =
      finding.evidence.find((item) => evidenceKey === `${item.line}:${item.text}`) ?? null;
    if (choice === "violation" && evidence === null) {
      setMessage(new Error("Choose the evidence line that supports the violation."));
      return;
    }
    const body: {
      expectedRevision: number;
      verdict: "pass" | "violation";
      reason: string;
      evidence?: Evidence;
    } = {
      expectedRevision: expectedFindingRevision(finding),
      verdict: choice,
      reason: reason.trim(),
    };
    if (choice === "violation" && evidence !== null) body.evidence = evidence;
    setPending(true);
    try {
      await decideFinding(labId, finding.id, body, session.csrfToken, crypto.randomUUID());
      await queryClient.invalidateQueries({ queryKey: ["live", "reviews", labId] });
      setMessage(new Error("Decision saved. No marks were published."));
    } catch (error) {
      setMessage(error);
    } finally {
      setPending(false);
    }
  }

  const verdictTone =
    finding.checkerVerdict === "pass"
      ? "ok"
      : finding.checkerVerdict === "violation"
        ? "bad"
        : "warn";
  const evidenceLines = new Set(finding.evidence.map((item) => item.line));
  const sourceLines = finding.source.split("\n");
  return (
    <article className="live-panel live-finding">
      <header>
        <h3>
          {finding.constraintId} · {finding.checkerVerdict}
        </h3>
        <span className={`live-status live-status--${verdictTone}`}>
          Checker {finding.checkerVerdict}
        </span>
        {finding.decision ? (
          <span className="live-status live-status--info">TA {finding.decision.verdict}</span>
        ) : null}
        <span className="live-finding-meta">
          {finding.requiredReview ? "Review required." : "Review not required."} Configured
          deduction: <strong className="live-num">{configured}</strong>
        </span>
      </header>
      <div className="live-finding-body">
        <div className="live-finding-code">
          {finding.unsureCause ? (
            <p className="live-callout">Unsure cause: {finding.unsureCause}</p>
          ) : null}
          <section aria-label="Evidence lines">
            <h4 className="live-label">Evidence</h4>
            {finding.evidence.length === 0 ? (
              <p className="live-muted">No evidence lines were returned.</p>
            ) : (
              <ol className="live-evidence">
                {finding.evidence.map((item) => (
                  <li key={`${item.line}:${item.text}`}>
                    <span>Line {item.line}:</span> <code>{item.text}</code>
                  </li>
                ))}
              </ol>
            )}
          </section>
          <pre className="live-source" tabIndex={0} aria-label="Submitted source">
            {sourceLines.map((line, index) => (
              <span
                key={index}
                className={evidenceLines.has(index + 1) ? "live-source-hit" : undefined}
              >
                <span className="live-source-num" aria-hidden="true">
                  {index + 1}
                </span>
                {line}
                {"\n"}
              </span>
            ))}
          </pre>
        </div>
        <form onSubmit={(event) => void onSubmit(event)}>
          <SavedTaDecision decision={finding.decision} deduction={configured} />
          <fieldset className="live-fieldset">
            <legend>Decision</legend>
            <div className="live-segment">
              {(["pass", "violation", "unsure"] as const).map((option) => (
                <label key={option} htmlFor={`${finding.id}-${option}`} data-option={option}>
                  <input
                    id={`${finding.id}-${option}`}
                    type="radio"
                    name={`decision-${finding.id}`}
                    value={option}
                    checked={choice === option}
                    onChange={() => setChoice(option)}
                  />
                  {option === "pass" ? "Pass" : option === "violation" ? "Violation" : "Unsure"}
                </label>
              ))}
            </div>
          </fieldset>
          <p className="live-muted">{chosen}</p>
          {choice === "violation" ? (
            <fieldset className="live-fieldset">
              <legend>Violation evidence</legend>
              <div className="live-evidence-pick">
                {finding.evidence.map((item) => {
                  const key = `${item.line}:${item.text}`;
                  return (
                    <label key={key} htmlFor={`${finding.id}-evidence-${key}`}>
                      <input
                        id={`${finding.id}-evidence-${key}`}
                        type="radio"
                        name={`evidence-${finding.id}`}
                        checked={evidenceKey === key}
                        onChange={() => setEvidenceKey(key)}
                      />
                      Line {item.line}
                    </label>
                  );
                })}
              </div>
            </fieldset>
          ) : null}
          <label className="live-field" htmlFor={`${finding.id}-reason`}>
            Reason
            <textarea
              id={`${finding.id}-reason`}
              rows={3}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          </label>
          {message ? <LiveError error={message} /> : null}
          <button className="live-button" type="submit" disabled={pending}>
            {pending ? "Saving…" : "Save decision"}
          </button>
        </form>
      </div>
    </article>
  );
}
