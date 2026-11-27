"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";

import { getCurrentUser } from "../lib/live/api";
import { useLiveSession } from "./live/live-shell";
import { Icon } from "./icon";

const destinations = [
  { href: "/", label: "Labs", icon: "labs", number: "01" },
  { href: "/live/classes", label: "Classes", icon: "classes", number: "02" },
  { href: "/publication", label: "Release results", icon: "review", number: "03" },
] as const;

export function WorkspaceNavigation() {
  const pathname = usePathname();
  return (
    <nav className="workspace-nav" aria-label="Workspace">
      <p className="nav-label">Workspace</p>
      {destinations.map((item) => {
        const active =
          item.href === "/"
            ? pathname === "/" ||
              pathname.startsWith("/live/labs/") ||
              pathname.startsWith("/live/submissions/")
            : pathname.startsWith(item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`workspace-nav-item${active ? " is-active" : ""}`}
            aria-current={active ? "page" : undefined}
          >
            <Icon name={item.icon} />
            <span>{item.label}</span>
            <small aria-hidden="true">{item.number}</small>
          </Link>
        );
      })}
      <div className="nav-note">
        <span className="accent-rule" />
        <p>
          Think it through.
          <br />
          <strong>Build your solution.</strong>
        </p>
        <small>C++17 assessment workspace</small>
      </div>
    </nav>
  );
}

export function AccountNavigation() {
  const session = useLiveSession();
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: session.ready && !session.closing && session.csrfToken !== null,
    retry: false,
    gcTime: 0,
    staleTime: 30_000,
  });
  const user = me.data?.user;
  return user === undefined || session.closing || session.csrfToken === null ? (
    <Link className="header-sign-in" href="/live">
      Sign in <Icon name="arrow" size={16} />
    </Link>
  ) : (
    <div className="account-controls">
      <Link className="account-navigation" href="/live/classes">
        <span className="account-avatar" aria-hidden="true">
          {user.displayName.slice(0, 1).toUpperCase()}
        </span>
        <span>
          {user.displayName}
          <small>{user.role}</small>
        </span>
      </Link>
      <button
        className="header-sign-out"
        type="button"
        disabled={session.closing}
        onClick={() => void session.logout()}
      >
        Sign out
      </button>
    </div>
  );
}
