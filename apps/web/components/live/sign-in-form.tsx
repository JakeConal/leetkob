"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { FormEvent } from "react";

import { ApiProblem, getCurrentUser, login } from "../../lib/live/api";
import { LiveError } from "./live-status";
import { useLiveSession } from "./live-shell";

export function SignInForm() {
  const session = useLiveSession();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [formError, setFormError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing,
    retry: false,
    gcTime: 0,
    staleTime: 0,
  });

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    setPending(true);
    try {
      const signedIn = await login(email, password);
      session.acceptSession(signedIn);
      router.push("/live/classes");
    } catch (error) {
      setFormError(error);
    } finally {
      setPending(false);
    }
  }

  if (!session.ready || session.closing) {
    return <p className="live-muted">{session.closing ? "Signing out…" : "Loading…"}</p>;
  }

  const signedOut = me.error instanceof ApiProblem && me.error.status === 401;
  return (
    <div className="live-stack">
      {session.banner ? <LiveError error={session.banner} /> : null}
      {me.isPending ? <p className="live-muted">Checking the current session…</p> : null}
      {me.isError && !signedOut ? <LiveError error={me.error} /> : null}
      {me.data ? (
        <section className="live-card" aria-labelledby="signed-in-heading">
          <h1 id="signed-in-heading">Signed in</h1>
          <p>
            {me.data.user.displayName} · {me.data.user.role}
          </p>
          <Link className="live-button" href="/live/classes">
            Continue to classes
          </Link>
        </section>
      ) : null}
      {!me.data && !me.isPending ? (
        <form className="live-card live-form" onSubmit={(event) => void onSubmit(event)}>
          <h1>Sign in</h1>
          <p className="live-muted">Use the synthetic assessment account for this class.</p>
          {formError ? <LiveError error={formError} /> : null}
          <label className="live-field" htmlFor="email">
            Email
            <input
              id="email"
              name="email"
              type="email"
              autoComplete="username"
              required
              value={email}
              aria-invalid={formError ? true : undefined}
              onChange={(event) => setEmail(event.target.value)}
            />
          </label>
          <label className="live-field" htmlFor="password">
            Password
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              aria-invalid={formError ? true : undefined}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>
          <button className="live-button" type="submit" disabled={pending}>
            {pending ? "Signing in…" : "Sign in"}
          </button>
        </form>
      ) : null}
    </div>
  );
}
