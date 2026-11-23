import type { ReactNode } from "react";
import { Plus_Jakarta_Sans } from "next/font/google";

import { LiveShell } from "../../components/live/live-shell";
import "./live.css";

const plusJakarta = Plus_Jakarta_Sans({
  subsets: ["latin"],
  display: "swap",
});

export const metadata = {
  title: "LeeTKOB live",
  description: "Sign in, author a lab, submit code, and review findings.",
};

export const dynamic = "force-dynamic";

export default function LiveLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div className={`live-workspace ${plusJakarta.className}`}>
      <LiveShell>{children}</LiveShell>
    </div>
  );
}
