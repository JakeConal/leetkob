"use client";

import { useParams } from "next/navigation";

import { SubmissionScreen } from "../../../../components/live/submission-screen";

export default function LiveSubmissionPage() {
  const params = useParams<{ submissionId: string }>();
  const submissionId = typeof params.submissionId === "string" ? params.submissionId : "";
  if (submissionId.length === 0) {
    return <p className="live-muted">This submission address is not valid.</p>;
  }
  return <SubmissionScreen submissionId={submissionId} />;
}
