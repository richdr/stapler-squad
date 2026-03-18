import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Configuration - Claude Squad",
  description: "Configure Claude Squad settings including agent programs, tmux prefix, and log levels.",
};

export default function ConfigLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
