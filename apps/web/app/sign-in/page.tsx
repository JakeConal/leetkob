import type { Metadata } from "next";

import { AppShell } from "../../components/app-shell";
import { SignInForm } from "./sign-in-form";

export const metadata: Metadata = {
  title: "Sign in | LeeTKOB",
  description: "Sign in to the PA#3 assessment workspace with a synthetic account.",
};

export default function SignInPage() {
  return (
    <AppShell
      rail={
        <section className="rail-block" aria-labelledby="session-heading">
          <p className="rail-index" aria-hidden="true">
            02
          </p>
          <h2 id="session-heading">Session boundary</h2>
          <div className="check-list">
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>HttpOnly cookie</strong>
                <small>Set by the API through this same-origin proxy.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Memory-only proof</strong>
                <small>CSRF token and protected reads never touch web storage.</small>
              </span>
            </div>
            <div className="check-row">
              <span className="status-mark" aria-hidden="true" />
              <span>
                <strong>Cleared on change</strong>
                <small>Sign-out or another account wipes protected state.</small>
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
          <h1 id="page-heading">Sign in</h1>
          <p>
            Use one of the synthetic PA#3 assessment accounts. The server decides the role and class
            access from its own records.
          </p>
        </div>
      </section>

      <SignInForm />
    </AppShell>
  );
}
