"use client";

import { useRouter } from "next/navigation";
import { SessionWizard } from "@/components/sessions/SessionWizard";
import { SessionFormData } from "@/lib/validation/sessionSchema";
import { useSessionService } from "@/lib/hooks/useSessionService";
import styles from "./page.module.css";

export default function NewSessionPage() {
  const router = useRouter();
  const { createSession } = useSessionService({
    baseUrl: "http://localhost:8543",
  });

  const handleComplete = async (data: SessionFormData) => {
    try {
      await createSession({
        title: data.title,
        path: data.path,
        workingDir: data.workingDir || "",
        branch: data.branch || "",
        program: data.program,
        category: data.category || "",
        prompt: data.prompt || "",
        autoYes: data.autoYes,
        existingWorktree: data.existingWorktree || "",
      });

      // Navigate back to home page
      router.push("/");
    } catch (error) {
      console.error("Failed to create session:", error);
      throw error;
    }
  };

  const handleCancel = () => {
    router.push("/");
  };

  return (
    <div className={styles.page}>
      <div className={styles.container}>
        <div className={styles.header}>
          <h1>Create New Session</h1>
          <p>Set up a new AI coding session with custom configuration</p>
        </div>
        <SessionWizard onComplete={handleComplete} onCancel={handleCancel} />
      </div>
    </div>
  );
}
