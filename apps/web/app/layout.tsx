import type { Metadata } from "next";
import type { ReactNode } from "react";

import { LiveShell } from "../components/live/live-shell";
import "./globals.css";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: "LeeTKOB | Practice workspace",
  description:
    "Your class labs, C++17 solutions, and TA-reviewed feedback in one focused workspace.",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <Providers>
          <LiveShell>{children}</LiveShell>
        </Providers>
      </body>
    </html>
  );
}
