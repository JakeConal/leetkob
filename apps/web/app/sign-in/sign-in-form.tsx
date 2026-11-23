"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { summarizeError, type ApiErrorSummary } from "../../lib/api-client";
import { signIn } from "../../lib/session-client";

export function SignInForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [problem, setProblem] = useState<ApiErrorSummary | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setPending(true);
    setProblem(null);
    try {
      await signIn(email, password);
      setPassword("");
      router.push("/classes");
    } catch (error) {
      setProblem(summarizeError(error));
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="panel" aria-labelledby="sign-in-heading">
      <div className="panel-heading">
        <h2 id="sign-in-heading">Assessment account</h2>
        <p>Sign-in accepts JSON only and fails with one generic message.</p>
      </div>

      <form className="form-stack" onSubmit={handleSubmit} noValidate>
        <div className="field">
          <label htmlFor="email">Email</label>
          <input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            inputMode="email"
            spellCheck={false}
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            aria-describedby="email-hint"
          />
          <p className="field-hint" id="email-hint">
            For example <code>ta1@leetkob.local</code>.
          </p>
        </div>

        <div className="field">
          <label htmlFor="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            aria-describedby="password-hint"
          />
          <p className="field-hint" id="password-hint">
            Passwords are printed once by the provisioning command.
          </p>
        </div>

        {problem === null ? null : (
          <div className="problem-alert" role="alert">
            <p className="problem-alert-title">{problem.title}</p>
            <p>{problem.detail}</p>
            <p className="problem-meta">
              {problem.status === null ? null : <span>HTTP {problem.status}</span>}
              {problem.type === null ? null : <span>{problem.type}</span>}
              {problem.retryAfterSeconds === null ? null : (
                <span>Retry after {problem.retryAfterSeconds} s</span>
              )}
            </p>
          </div>
        )}

        <div className="actions">
          <button className="primary-action" type="submit" disabled={pending}>
            {pending ? "Signing in…" : "Sign in"}
          </button>
          <Link className="inline-action" href="/">
            Publication control
          </Link>
        </div>
      </form>
    </section>
  );
}
