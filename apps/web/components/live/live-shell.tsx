"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { logout as logoutRequest } from "../../lib/live/api";
import type { UserSession } from "../../lib/live/api";
import { clearProtectedSession, readCsrfToken, writeCsrfToken } from "../../lib/live/session";

interface LiveSessionValue {
  readonly ready: boolean;
  readonly closing: boolean;
  readonly csrfToken: string | null;
  readonly banner: unknown;
  readonly acceptSession: (session: UserSession) => void;
  readonly logout: () => Promise<void>;
  readonly clearBanner: () => void;
}

const LiveSessionContext = createContext<LiveSessionValue | null>(null);

export function useLiveSession(): LiveSessionValue {
  const value = useContext(LiveSessionContext);
  if (value === null) throw new Error("Live session is unavailable.");
  return value;
}

export function LiveShell({ children }: { readonly children: ReactNode }) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [closing, setClosing] = useState(false);
  const [csrfToken, setCsrfToken] = useState<string | null>(null);
  const [banner, setBanner] = useState<unknown>(null);

  useEffect(() => {
    setCsrfToken(readCsrfToken(window.sessionStorage));
    setReady(true);
  }, []);

  const acceptSession = useCallback(
    (session: UserSession) => {
      queryClient.clear();
      writeCsrfToken(window.sessionStorage, session.csrfToken);
      setCsrfToken(session.csrfToken);
      setBanner(null);
      setClosing(false);
    },
    [queryClient],
  );

  const logout = useCallback(async () => {
    const token = csrfToken ?? readCsrfToken(window.sessionStorage);
    setClosing(true);
    setCsrfToken(null);
    clearProtectedSession(window.sessionStorage, queryClient);
    router.replace("/live");
    try {
      if (token !== null) await logoutRequest(token);
      setBanner(null);
    } catch (error) {
      setBanner(error);
    } finally {
      setClosing(false);
    }
  }, [csrfToken, queryClient, router]);

  const clearBanner = useCallback(() => setBanner(null), []);

  const value = useMemo(
    () => ({ ready, closing, csrfToken, banner, acceptSession, logout, clearBanner }),
    [ready, closing, csrfToken, banner, acceptSession, logout, clearBanner],
  );

  return (
    <LiveSessionContext.Provider value={value}>
      <a className="live-skip" href="#live-content">
        Skip to content
      </a>
      <div className="live-shell">
        <nav className="live-nav" aria-label="Live workspace">
          <p className="live-wordmark">LeeTKOB</p>
          <Link href="/live">Sign in</Link>
          <Link href="/live/classes">Classes</Link>
          <Link href="/">PA#2 home</Link>
          <button type="button" onClick={() => void logout()} disabled={closing}>
            Log out
          </button>
        </nav>
        <main id="live-content" className="live-main">
          {children}
        </main>
      </div>
    </LiveSessionContext.Provider>
  );
}
