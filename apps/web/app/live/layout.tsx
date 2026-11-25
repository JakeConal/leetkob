import type { ReactNode } from "react";

export const metadata = {
  title: "LeeTKOB live",
  description: "Sign in, author a lab, submit code, and review findings.",
};

export const dynamic = "force-dynamic";

export default function LiveLayout({ children }: Readonly<{ children: ReactNode }>) {
  return children;
}
