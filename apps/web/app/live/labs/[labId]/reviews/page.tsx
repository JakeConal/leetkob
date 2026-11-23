"use client";

import { useParams } from "next/navigation";

import { ReviewQueue } from "../../../../../components/live/review-queue";

export default function LiveReviewPage() {
  const params = useParams<{ labId: string }>();
  const labId = typeof params.labId === "string" ? params.labId : "";
  if (labId.length === 0) return <p className="live-muted">This lab address is not valid.</p>;
  return <ReviewQueue labId={labId} />;
}
