import type { Metadata } from "next";

import { AppShell } from "../../../components/app-shell";
import { LabDetailPanel } from "./lab-detail";
import { SubmissionPanel } from "./submission-panel";

export const metadata: Metadata = {
  title: "Lab | LeeTKOB",
  description: "Safe lab detail with constraints, limits and sample tests.",
};

export default async function LabPage({
  params,
}: Readonly<{ params: Promise<{ labId: string }> }>) {
  const { labId } = await params;

  return (
    <AppShell
      rail={
        <section className="rail-block" aria-labelledby="lab-rules-heading">
          <p className="rail-index" aria-hidden="true">
            02
          </p>
          <h2 id="lab-rules-heading">Safe projection</h2>
          <div className="check-list">
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Sample tests only</strong>
                <small>Hidden inputs, expected outputs and checker findings never serialize.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>TA-configured deductions</strong>
                <small>The model never chooses a deduction.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Concealed not found</strong>
                <small>Another class&apos;s lab answers like an unknown ID.</small>
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
          <h1 id="page-heading">Lab detail</h1>
          <p>
            Statement, constraints and limits as students may read them, with only the tests marked
            as samples.
          </p>
        </div>
      </section>

      <LabDetailPanel labId={labId} />
      <SubmissionPanel labId={labId} />
    </AppShell>
  );
}
