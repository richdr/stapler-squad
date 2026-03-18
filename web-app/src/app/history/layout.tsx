import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "History - Claude Squad",
  description: "Browse conversation history and session transcripts from your Claude Squad agents.",
};

export default function HistoryLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
