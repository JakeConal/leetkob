"use client";

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { fetchHealth } from "../lib/health";

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
    <div className="check-row">
      <span className={`status-mark status-mark--${state}`} aria-hidden="true" />
      <span>
        <strong>{label}</strong>
        <small>{detail}</small>
      </span>
    </div>
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

  let problemDetails: ReactNode = (
    <div className="problem-message">
      <p>No system problems detected.</p>
    </div>
  );
  if (health.isPending) {
    problemDetails = (
      <div className="problem-message">
        <p>Checking system status…</p>
      </div>
    );
  } else if (health.isError) {
    problemDetails = (
      <div className="problem-message problem-message--error">
        <p>Failed to connect to API.</p>
        <button type="button" onClick={() => void health.refetch()}>
          Retry
        </button>
      </div>
    );
  }

  return (
    <aside className="status-rail" aria-labelledby="system-checks-heading">
      <div className="rail-index" aria-hidden="true">
        02
      </div>
      <h2 id="system-checks-heading">System checks</h2>
      <div className="check-list" aria-live="polite">
        <StatusRow label="API status" detail={apiDetail} state={apiState} />
        <StatusRow label="Contract tests" detail="Configured" />
        <StatusRow label="Identity verification" detail="Signed claims" />
      </div>

      <section className="problem-region" aria-labelledby="problem-heading">
        <h3 id="problem-heading">Problem details</h3>
        {problemDetails}
      </section>
    </aside>
  );
}
