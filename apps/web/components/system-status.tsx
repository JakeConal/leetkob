"use client";

import { useQuery } from "@tanstack/react-query";

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

  const apiState = health.isPending ? "loading" : health.isError ? "error" : "ready";
  const apiDetail = health.isPending ? "Checking" : health.isError ? "Unavailable" : "Connected";

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
        {health.isError ? (
          <div className="problem-message problem-message--error">
            <p>Failed to connect to API.</p>
            <button type="button" onClick={() => void health.refetch()}>
              Retry
            </button>
          </div>
        ) : health.isPending ? (
          <div className="problem-message">
            <p>Checking system status…</p>
          </div>
        ) : (
          <div className="problem-message">
            <p>No system problems detected.</p>
          </div>
        )}
      </section>
    </aside>
  );
}
