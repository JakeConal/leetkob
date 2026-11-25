"use client";

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { fetchHealth } from "../lib/health";

const STATE_CLASS = {
  ready: "live-status--ok",
  loading: "live-status--warn",
  error: "live-status--bad",
} as const;

function StatusRow({
  detail,
  label,
  state = "ready",
}: Readonly<{
  detail: string;
  label: string;
  state?: "error" | "loading" | "ready";
}>) {
  return (
    <li>
      <strong>{label}</strong>
      <span className={`live-status ${STATE_CLASS[state]}`}>{detail}</span>
    </li>
  );
}

export function SystemStatus() {
  const health = useQuery({
    queryKey: ["api-health"],
    queryFn: () => fetchHealth(),
    refetchInterval: 60_000,
  });

  let apiState: "error" | "loading" | "ready" = "ready";
  let apiDetail = "Connected";
  if (health.isPending) {
    apiState = "loading";
    apiDetail = "Checking";
  } else if (health.isError) {
    apiState = "error";
    apiDetail = "Unavailable";
  }

  let problemDetails: ReactNode = <p className="live-muted">No system problems detected.</p>;
  if (health.isPending) {
    problemDetails = <p className="live-loading">Checking system status…</p>;
  } else if (health.isError) {
    problemDetails = (
      <div className="live-stack-sm">
        <p className="live-alert" role="alert">
          Failed to connect to API.
        </p>
        <button
          className="live-button live-button-secondary live-button-sm"
          type="button"
          onClick={() => void health.refetch()}
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <aside className="live-panel live-checks" aria-labelledby="system-checks-heading">
      <h2 id="system-checks-heading">System checks</h2>
      <ul aria-live="polite">
        <StatusRow label="API status" detail={apiDetail} state={apiState} />
        <StatusRow label="Contract tests" detail="Configured" />
        <StatusRow label="Identity verification" detail="Signed claims" />
      </ul>
      <section aria-labelledby="problem-heading">
        <h3 id="problem-heading">Problem details</h3>
        {problemDetails}
      </section>
    </aside>
  );
}
