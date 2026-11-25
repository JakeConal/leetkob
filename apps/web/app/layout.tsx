import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Archivo, Geist, Geist_Mono, JetBrains_Mono } from "next/font/google";

import { LiveShell } from "../components/live/live-shell";
import "./globals.css";
import "./live/live.css";
import "./live/theme.css";
import { Providers } from "./providers";

const geist = Geist({ subsets: ["latin"], display: "swap", variable: "--live-font-sans" });
const geistMono = Geist_Mono({ subsets: ["latin"], display: "swap", variable: "--live-font-mono" });
const archivo = Archivo({ subsets: ["latin"], display: "swap", variable: "--font-archivo" });
const jetbrains = JetBrains_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-jetbrains",
});

export const metadata: Metadata = {
  title: "LeeTKOB | Publication control",
  description: "Reviewer-facing publication gate for the LeeTKOB lab workflow.",
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body
        className={`${geist.variable} ${geistMono.variable} ${archivo.variable} ${jetbrains.variable}`}
      >
        <Providers>
          <div className="live-workspace live-press">
            <LiveShell>{children}</LiveShell>
          </div>
        </Providers>
      </body>
    </html>
  );
}
