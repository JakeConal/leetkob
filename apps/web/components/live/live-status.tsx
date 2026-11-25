import React from "react";

import { ApiConnectionError, ApiProblem, ContractError } from "../../lib/live/api";

void React;

export function LiveError({ error }: { readonly error: unknown }) {
  const message =
    error instanceof ApiProblem ||
    error instanceof ApiConnectionError ||
    error instanceof ContractError ||
    error instanceof Error
      ? error.message
      : "The request could not be completed.";
  return (
    <p className="live-alert" role="alert">
      {message}
    </p>
  );
}

export function LiveNotFound() {
  return (
    <section className="live-panel live-not-found" aria-labelledby="not-found-heading">
      <h1 id="not-found-heading">Not found</h1>
      <p className="live-muted">The requested resource does not exist.</p>
      <a className="live-button" href="/live">
        Return to sign in
      </a>
    </section>
  );
}
