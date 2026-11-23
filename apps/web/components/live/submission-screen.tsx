"use client";

import { useQuery } from "@tanstack/react-query";

import { readSubmission } from "../../lib/live/api";
import { projectStudentStatus } from "../../lib/live/view-rules";
import { useReplaceNotFound } from "./live-feedback";
import { LiveError, LiveNotFound } from "./live-status";
import { useLiveSession } from "./live-shell";
import { StudentSubmissionStatus } from "./student-submission-status";

export function SubmissionScreen({ submissionId }: { readonly submissionId: string }) {
  const session = useLiveSession();
  const submission = useQuery({
    queryKey: ["live", "submission", submissionId],
    queryFn: () => readSubmission(submissionId),
    enabled: session.ready && !session.closing,
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchInterval: (query) => {
      if (query.state.error) return false;
      try {
        const status = projectStudentStatus(query.state.data);
        return status.state === "judged" ? false : 2000;
      } catch {
        return false;
      }
    },
  });
  const notFound = useReplaceNotFound(submission.error);
  if (!session.ready || session.closing) return <p className="live-muted">Loading…</p>;
  if (notFound) return <LiveNotFound />;
  if (submission.isPending) return <p className="live-muted">Loading submission status…</p>;
  if (submission.isError) return <LiveError error={submission.error} />;
  if (submission.data === undefined) return null;
  return (
    <div className="live-stack">
      <StudentSubmissionStatus payload={submission.data} />
      <button
        className="live-button live-button-secondary"
        type="button"
        onClick={() => void submission.refetch()}
      >
        Refresh status
      </button>
    </div>
  );
}
