import React from "react";

import type { Finding } from "../../lib/live/api";

void React;

export function SavedTaDecision({
  decision,
  deduction,
}: {
  readonly decision: Finding["decision"];
  readonly deduction: number;
}) {
  if (decision === undefined) return null;
  return (
    <section className="live-callout" aria-label="Saved TA decision">
      <p>{`Saved TA decision: ${decision.verdict}. Confirmed deduction: ${decision.verdict === "violation" ? deduction : 0}.`}</p>
      <p>Reason: {decision.reason}</p>
    </section>
  );
}
