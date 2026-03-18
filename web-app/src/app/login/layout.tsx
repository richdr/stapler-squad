import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Sign In - Claude Squad",
  description: "Sign in to Claude Squad to manage your AI agent sessions.",
};

export default function LoginLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
