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

  if (!session.ready || session.closing) return <p className="live-muted">Loading…</p>;
  if (notFound) return <LiveNotFound />;
  if (reviews.isPending) {
    return (
      <div className="live-stack">
        <PublishControl loading />
        <p className="live-muted">Loading review queue…</p>
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

  return (
    <div className="live-stack">
      <PublishControl
        reviewPending={reviews.data.reviewPending}
        processingPending={reviews.data.processingPending}
        deadlinePassed={deadlinePassed}
      />
      <h1>Review queue</h1>
      {items.length === 0 ? <p>No findings are in this queue.</p> : null}
      {items.map((finding) => (
        <FindingReview key={finding.id} labId={labId} finding={finding} />
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
      ? "No deduction chosen yet."
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

  return (
    <article className="live-card live-finding">
      <header>
        <h2>
          {finding.constraintId} · {finding.checkerVerdict}
        </h2>
        {finding.unsureCause ? <p>Unsure cause: {finding.unsureCause}</p> : null}
        <p>{finding.requiredReview ? "Review required." : "Review not required."}</p>
        <p>Configured deduction: {configured}</p>
      </header>
      <section aria-label="Evidence lines">
        <h3>Evidence</h3>
        {finding.evidence.length === 0 ? <p>No evidence lines were returned.</p> : null}
        <ol>
          {finding.evidence.map((item) => (
            <li key={`${item.line}:${item.text}`}>
              Line {item.line}: <code>{item.text}</code>
            </li>
          ))}
        </ol>
      </section>
      <div className="live-panels">
        <pre className="live-source" tabIndex={0} aria-label="Submitted source">
          {finding.source}
        </pre>
        <form onSubmit={(event) => void onSubmit(event)}>
          <fieldset className="live-choice">
            <legend>Decision</legend>
            {(["pass", "violation", "unsure"] as const).map((option) => (
              <label key={option} htmlFor={`${finding.id}-${option}`}>
                <input
                  id={`${finding.id}-${option}`}
                  type="radio"
                  name={`decision-${finding.id}`}
                  value={option}
                  checked={choice === option}
                  onChange={() => setChoice(option)}
                />{" "}
                {option === "pass" ? "Pass" : option === "violation" ? "Violation" : "Unsure"}
              </label>
            ))}
          </fieldset>
          <p>{chosen}</p>
          {choice === "violation" ? (
            <fieldset className="live-choice">
              <legend>Violation evidence</legend>
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
                    />{" "}
                    Line {item.line}
                  </label>
                );
              })}
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
