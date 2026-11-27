import type { ReactNode } from "react";

import "./live.css";
import "./practice.css";

export const metadata = {
  title: "LeeTKOB live",
  description: "Sign in, author a lab, submit code, and review findings.",
};

export const dynamic = "force-dynamic";

export default function LiveLayout({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="live-workspace practice-tools">{children}</div>;
}
