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
      <section className="live-card" aria-labelledby="publication-heading">
        <h2 id="publication-heading">Publication blocked</h2>
        <p id="publication-explanation">Publication blocked. The review queue is still loading.</p>
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
    <section className="live-card" aria-labelledby="publication-heading">
      <h2 id="publication-heading">{gate.title}</h2>
      <p id="publication-explanation">{gate.explanation}</p>
      <p>{deadline}</p>
      <p>{gate.processingPending ? "Processing incomplete." : "Processing complete."}</p>
      <p>Pending reviews: {gate.reviewPending}</p>
      <button type="button" disabled={gate.disabled} aria-describedby="publication-explanation">
        Publish lab
      </button>
    </section>
  );
}
