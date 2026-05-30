import type { Metadata } from "next";

import { AuthGate } from "@/auth-gate";
import { AuthShell } from "@/auth-shell";
import { TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { Geist } from "next/font/google";

import "./globals.css";

const geist = Geist({ subsets: ["latin"], variable: "--font-sans" });

export const metadata: Metadata = {
  title: "SuperTeam Console",
  description: "SuperTeam external control-plane console",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="zh-CN" className={cn("font-sans", geist.variable)} suppressHydrationWarning>
      <body>
        <TooltipProvider>
          <AuthShell>
            <AuthGate>{children}</AuthGate>
          </AuthShell>
        </TooltipProvider>
      </body>
    </html>
  );
}
