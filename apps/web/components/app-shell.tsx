import type { ReactNode } from "react";

/**
 * Shared Swiss-minimalist page frame: masthead, twelve-column workspace,
 * primary reading column, optional index rail and a divided footer. The DOM
 * order is the reading order, so stacked narrow layouts keep the same order.
 */
export function AppShell({
  children,
  rail,
  environment = "Development / PA#3",
}: Readonly<{
  children: ReactNode;
  rail?: ReactNode;
  environment?: string;
}>) {
  return (
    <main className="page-shell">
      <header className="masthead">
        <a className="wordmark" href="/" aria-label="LeeTKOB home">
          LeeTKOB
        </a>
        <p className="environment">
          <span aria-hidden="true" /> {environment}
        </p>
      </header>

      <div className="workspace" id="main-content">
        <div className="primary-column">{children}</div>
        {rail === undefined ? null : <aside className="status-rail">{rail}</aside>}
      </div>

      <footer className="footer">
        <p>LeeTKOB assessment workspace</p>
        <p>PA#3 / Local development</p>
      </footer>
    </main>
  );
}
