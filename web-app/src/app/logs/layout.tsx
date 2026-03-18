import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Logs - Claude Squad",
  description: "View and search application logs from your Claude Squad sessions.",
};

export default function LogsLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
