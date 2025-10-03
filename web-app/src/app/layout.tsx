import type { Metadata } from "next";
import { Navigation } from "@/components/ui/Navigation";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import "./globals.css";

export const metadata: Metadata = {
  title: "Claude Squad Sessions",
  description: "Manage your AI agent sessions",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>
        <ErrorBoundary>
          <Navigation />
          <main>{children}</main>
        </ErrorBoundary>
      </body>
    </html>
  );
}
