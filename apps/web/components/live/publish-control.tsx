import React from "react";

import { livePublicationGate } from "../../lib/live/view-rules";

void React;

export function PublishControl(
  props:
    | { readonly loading: true }
    | {
        readonly reviewPending: number;
        readonly processingPending: boolean;
        readonly deadlinePassed: boolean | null;
      },
) {
  if ("loading" in props) {
    return (
      <section className="live-gate" aria-labelledby="publication-heading">
        <div>
          <h2 id="publication-heading">Publication blocked</h2>
          <p id="publication-explanation">
            Publication blocked. The review queue is still loading.
          </p>
        </div>
        <button type="button" disabled aria-describedby="publication-explanation">
          Publish lab
        </button>
      </section>
    );
  }

  const gate = livePublicationGate(props);
  const deadline =
    gate.deadlinePassed === null
      ? "Submission deadline has not been loaded."
      : gate.deadlinePassed
        ? "Submission deadline has passed."
        : "Submission deadline has not passed.";
  return (
    <section className="live-gate" aria-labelledby="publication-heading">
      <div>
        <h2 id="publication-heading">{gate.title}</h2>
        <p id="publication-explanation">{gate.explanation}</p>
        <ul className="live-gate-facts">
          <li data-ok={gate.deadlinePassed === null ? undefined : String(gate.deadlinePassed)}>
            {deadline}
          </li>
          <li data-ok={String(!gate.processingPending)}>
            {gate.processingPending ? "Processing incomplete." : "Processing complete."}
          </li>
          <li data-ok={String(gate.reviewPending === 0)}>Pending reviews: {gate.reviewPending}</li>
        </ul>
      </div>
      <button type="button" disabled={gate.disabled} aria-describedby="publication-explanation">
        Publish lab
      </button>
    </section>
  );
}
