"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { getCurrentUser, logout as logoutRequest } from "../../lib/live/api";
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

function NavIcon({ path }: { readonly path: string }) {
  return (
    <svg
      className="live-icon"
      viewBox="0 0 24 24"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={path} />
    </svg>
  );
}

const ROLE_LABEL = { admin: "Admin", ta: "Teaching assistant", student: "Student" } as const;

function SessionIdentity({ enabled }: { readonly enabled: boolean }) {
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled,
    retry: false,
    gcTime: 0,
    staleTime: 0,
  });
  if (!me.data) return null;
  return (
    <p className="live-identity">
      <span>{me.data.user.displayName}</span>
      <span>{ROLE_LABEL[me.data.user.role]}</span>
    </p>
  );
}

export function LiveShell({ children }: { readonly children: ReactNode }) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const onSignIn = pathname === "/live";
  const onClasses = !onSignIn && pathname.startsWith("/live/");
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
          <div className="live-nav-links">
            <Link href="/live" aria-current={onSignIn ? "page" : undefined}>
              <NavIcon path="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4M10 17l5-5-5-5M15 12H3" />
              Sign in
            </Link>
            <Link href="/live/classes" aria-current={onClasses ? "page" : undefined}>
              <NavIcon path="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V3H6.5A2.5 2.5 0 0 0 4 5.5v14zM4 19.5A2.5 2.5 0 0 0 6.5 22H20v-5" />
              Classes
            </Link>
            <Link href="/" aria-current={pathname === "/" ? "page" : undefined}>
              <NavIcon path="M3 10.5 12 3l9 7.5V21h-6v-6H9v6H3z" />
              PA#2 home
            </Link>
          </div>
          <div className="live-nav-foot">
            <SessionIdentity enabled={ready && !closing && pathname.startsWith("/live/")} />
            <button
              className="live-nav-logout"
              type="button"
              onClick={() => void logout()}
              disabled={closing}
            >
              <NavIcon path="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />
              Log out
            </button>
          </div>
        </nav>
        <main id="live-content" className="live-main">
          {children}
        </main>
      </div>
    </LiveSessionContext.Provider>
  );
}
