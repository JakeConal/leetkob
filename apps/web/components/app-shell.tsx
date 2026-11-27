import type { ReactNode } from "react";
import Link from "next/link";

import { Icon } from "./icon";
import { AccountNavigation, WorkspaceNavigation } from "./workspace-navigation";

/**
 * Shared Swiss-minimalist page frame: masthead, workspace navigation,
 * primary reading column, optional supporting rail and a divided footer. The DOM
 * order is the reading order, so stacked narrow layouts keep the same order.
 */
export function AppShell({
  children,
  rail,
  environment = "Assessment workspace",
  wide = false,
}: Readonly<{
  children: ReactNode;
  rail?: ReactNode;
  environment?: string;
  wide?: boolean;
}>) {
  return (
    <div className={`page-shell${wide ? " page-shell--wide" : ""}`}>
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <header className="masthead">
        <Link className="wordmark" href="/" aria-label="LeeTKOB home">
          <span className="brand-mark">
            <Icon name="code" size={22} />
          </span>
          LeeTKOB<span className="brand-period">.</span>
        </Link>
        <p className="environment">{environment}</p>
        <AccountNavigation />
      </header>
      <div className="app-frame">
        <WorkspaceNavigation />
        <main
          className={`workspace${rail === undefined ? " workspace--full" : ""}`}
          id="main-content"
          tabIndex={-1}
        >
          <div className="primary-column">{children}</div>
          {rail === undefined ? null : <aside className="status-rail">{rail}</aside>}
        </main>
      </div>
      <footer className="footer">
        <p>
          LeeTKOB <span>/</span> Learn through practice.
        </p>
        <p>
          C++17 <span>·</span> Model-assisted. TA-reviewed.
        </p>
      </footer>
    </div>
  );
}
