"use client";

import { useState } from "react";
import { SessionList, SessionCreateForm } from "@/components/sessions";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { Session } from "@/gen/session/v1/types_pb";
import styles from "./page.module.css";

export default function SessionsPage() {
  const {
    sessions,
    loading,
    error,
    createSession,
    deleteSession,
    pauseSession,
    resumeSession,
  } = useSessionService({
    autoWatch: true, // Enable real-time updates
  });

  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [showCreateForm, setShowCreateForm] = useState(false);

  const handleSessionClick = (session: Session) => {
    setSelectedSession(session);
    // TODO: Navigate to session detail page or open terminal overlay
  };

  const handleDeleteSession = async (sessionId: string) => {
    if (confirm("Are you sure you want to delete this session?")) {
      await deleteSession(sessionId);
    }
  };

  const handlePauseSession = async (sessionId: string) => {
    await pauseSession(sessionId);
  };

  const handleResumeSession = async (sessionId: string) => {
    await resumeSession(sessionId);
  };

  const handleCreateSession = async (request: Parameters<typeof createSession>[0]) => {
    const session = await createSession(request);
    if (session) {
      setShowCreateForm(false);
    }
  };

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1 className={styles.title}>Claude Squad Sessions</h1>
        <button
          className={styles.createButton}
          onClick={() => setShowCreateForm(true)}
        >
          + Create Session
        </button>
      </header>

      {error && (
        <div className={styles.error}>
          <p>Error: {error.message}</p>
          <button onClick={() => window.location.reload()}>Retry</button>
        </div>
      )}

      {loading && sessions.length === 0 ? (
        <div className={styles.loading}>
          <div className={styles.spinner} />
          <p>Loading sessions...</p>
        </div>
      ) : (
        <SessionList
          sessions={sessions}
          onSessionClick={handleSessionClick}
          onDeleteSession={handleDeleteSession}
          onPauseSession={handlePauseSession}
          onResumeSession={handleResumeSession}
        />
      )}

      {selectedSession && (
        <div className={styles.detailPanel}>
          <div className={styles.detailHeader}>
            <h2>{selectedSession.title}</h2>
            <button onClick={() => setSelectedSession(null)}>✕</button>
          </div>
          <div className={styles.detailContent}>
            <p>Session details will appear here</p>
            {/* TODO: Add session detail view with terminal, diff, etc. */}
          </div>
        </div>
      )}

      {showCreateForm && (
        <SessionCreateForm
          onSubmit={handleCreateSession}
          onCancel={() => setShowCreateForm(false)}
        />
      )}
    </div>
  );
}
