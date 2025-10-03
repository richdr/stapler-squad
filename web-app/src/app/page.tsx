"use client";

import { useState } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionList } from "@/components/sessions/SessionList";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetail } from "@/components/sessions/SessionDetail";
import { ErrorState } from "@/components/ui/ErrorState";
import { KeyboardHints } from "@/components/ui/KeyboardHint";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import styles from "./page.module.css";

export default function Home() {
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const {
    sessions,
    loading,
    error,
    deleteSession,
    pauseSession,
    resumeSession,
    listSessions,
  } = useSessionService({
    baseUrl: "http://localhost:8543",
    autoWatch: true,
  });

  // Keyboard shortcuts
  useKeyboard({
    "?": () => setShowHelp(true),
    Escape: () => {
      if (showHelp) {
        setShowHelp(false);
      } else if (selectedSession) {
        setSelectedSession(null);
      }
    },
    "r": () => !loading && listSessions(),
  });

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        {loading && <SessionListSkeleton count={4} />}
        {error && !loading && (
          <ErrorState
            error={error}
            title="Failed to Load Sessions"
            message="Unable to connect to the server. Please check that the server is running and try again."
            onRetry={() => listSessions()}
          />
        )}
        {!loading && !error && (
          <SessionList
            sessions={sessions}
            onSessionClick={(session) => setSelectedSession(session)}
            onDeleteSession={deleteSession}
            onPauseSession={pauseSession}
            onResumeSession={resumeSession}
          />
        )}
      </main>

      {/* Session detail modal */}
      {selectedSession && (
        <div className={styles.modal} onClick={() => setSelectedSession(null)}>
          <div className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <SessionDetail
              session={selectedSession}
              onClose={() => setSelectedSession(null)}
            />
          </div>
        </div>
      )}

      {/* Keyboard shortcuts help modal */}
      {showHelp && (
        <div className={styles.modal} onClick={() => setShowHelp(false)}>
          <div className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.modalHeader}>
              <h2>Keyboard Shortcuts</h2>
              <button
                className={styles.closeButton}
                onClick={() => setShowHelp(false)}
                aria-label="Close"
              >
                ✕
              </button>
            </div>
            <div className={styles.modalBody}>
              <KeyboardHints
                hints={[
                  { keys: "?", description: "Show keyboard shortcuts" },
                  { keys: "Escape", description: "Close modal / dialog" },
                  { keys: "r", description: "Refresh session list" },
                  { keys: "Enter", description: "Open selected session" },
                  { keys: "/", description: "Focus search (coming soon)" },
                  { keys: ["↑", "↓"], description: "Navigate sessions (coming soon)" },
                ]}
              />
            </div>
          </div>
        </div>
      )}

      {/* Floating help button */}
      <button
        className={styles.helpButton}
        onClick={() => setShowHelp(true)}
        aria-label="Show keyboard shortcuts"
        title="Keyboard shortcuts (?)"
      >
        ?
      </button>
    </div>
  );
}
