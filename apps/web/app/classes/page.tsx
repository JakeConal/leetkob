import type { Metadata } from "next";

import { AppShell } from "../../components/app-shell";
import { ClassList } from "./class-list";

export const metadata: Metadata = {
  title: "Classes | LeeTKOB",
  description: "Current synthetic account and its server-owned class memberships.",
};

export default function ClassesPage() {
  return (
    <AppShell
      rail={
        <section className="rail-block" aria-labelledby="access-heading">
          <p className="rail-index" aria-hidden="true">
            02
          </p>
          <h2 id="access-heading">Access rules</h2>
          <div className="check-list">
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Server-owned roles</strong>
                <small>Roles and memberships come from `/v2/auth/me`, never caller headers.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Assigned TAs author</strong>
                <small>Only the TA assigned to a class can create its labs.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Protected data clears</strong>
                <small>Sign-out and account changes discard cached class data.</small>
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
          <h1 id="page-heading">Classes</h1>
          <p>
            Read the current account and the classes it can reach. Protected reads stay in memory
            for this page only.
          </p>
        </div>
      </section>

      <ClassList />
    </AppShell>
  );
}
