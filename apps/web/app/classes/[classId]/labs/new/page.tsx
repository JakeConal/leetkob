import type { Metadata } from "next";

import { AppShell } from "../../../../../components/app-shell";
import { LabAuthoringForm } from "./lab-authoring-form";

export const metadata: Metadata = {
  title: "New lab | LeeTKOB",
  description: "Assigned-TA lab authoring through the PA#3 API.",
};

export default async function NewLabPage({
  params,
}: Readonly<{ params: Promise<{ classId: string }> }>) {
  const { classId } = await params;

  return (
    <AppShell
      rail={
        <section className="rail-block" aria-labelledby="authoring-rules-heading">
          <p className="rail-index" aria-hidden="true">
            02
          </p>
          <h2 id="authoring-rules-heading">Authoring rules</h2>
          <div className="check-list">
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Assigned TA only</strong>
                <small>The API verifies current membership and role.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>CSRF proof</strong>
                <small>Mutations carry the in-memory X-CSRF-Token.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Idempotent submit</strong>
                <small>Retries reuse one 128-bit Idempotency-Key until success.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Atomic storage</strong>
                <small>Tests are stored with the lab; a failure leaves no partial lab.</small>
              </span>
            </div>
          </div>
        </section>
      }
    >
      <section className="intro" aria-labelledby="page-heading">
        <div className="section-index" aria-hidden="true">
          01
        </div>
        <div>
          <h1 id="page-heading">Create lab</h1>
          <p>
            Author the statement, constraints, tests and limits. Client validation mirrors the
            documented bounds; the server remains authoritative.
          </p>
        </div>
      </section>

      <LabAuthoringForm classId={classId} />
    </AppShell>
  );
}
