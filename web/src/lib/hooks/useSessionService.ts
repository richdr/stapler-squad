"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import {
  Session,
  SessionStatus,
  CreateSessionRequest,
  UpdateSessionRequest,
  DeleteSessionRequest,
} from "@/gen/session/v1/session_pb";
import { SessionEvent } from "@/gen/session/v1/events_pb";

interface UseSessionServiceOptions {
  baseUrl?: string;
  autoWatch?: boolean;
}

interface UseSessionServiceReturn {
  // State
  sessions: Session[];
  loading: boolean;
  error: Error | null;

  // Methods
  listSessions: (options?: { category?: string; status?: SessionStatus }) => Promise<void>;
  getSession: (id: string) => Promise<Session | null>;
  createSession: (request: Partial<CreateSessionRequest>) => Promise<Session | null>;
  updateSession: (id: string, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;
  deleteSession: (id: string, force?: boolean) => Promise<boolean>;
  pauseSession: (id: string) => Promise<Session | null>;
  resumeSession: (id: string) => Promise<Session | null>;

  // Real-time updates
  watchSessions: (options?: { categoryFilter?: string; statusFilter?: SessionStatus }) => void;
  stopWatching: () => void;
}

export function useSessionService(
  options: UseSessionServiceOptions = {}
): UseSessionServiceReturn {
  const { baseUrl = "http://localhost:8080", autoWatch = false } = options;

  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient> | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl,
    });

    clientRef.current = createClient(SessionService, transport);
  }, [baseUrl]);

  // List sessions
  const listSessions = useCallback(
    async (listOptions?: { category?: string; status?: SessionStatus }) => {
      if (!clientRef.current) return;

      setLoading(true);
      setError(null);

      try {
        const response = await clientRef.current.listSessions({
          category: listOptions?.category,
          status: listOptions?.status,
        });

        setSessions(response.sessions);
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to list sessions"));
      } finally {
        setLoading(false);
      }
    },
    []
  );

  // Get single session
  const getSession = useCallback(async (id: string): Promise<Session | null> => {
    if (!clientRef.current) return null;

    try {
      const response = await clientRef.current.getSession({ id });
      return response.session ?? null;
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Failed to get session"));
      return null;
    }
  }, []);

  // Create session
  const createSession = useCallback(
    async (request: Partial<CreateSessionRequest>): Promise<Session | null> => {
      if (!clientRef.current) return null;

      setError(null);

      try {
        const response = await clientRef.current.createSession({
          title: request.title ?? "",
          path: request.path ?? "",
          workingDir: request.workingDir,
          branch: request.branch,
          program: request.program,
          category: request.category,
          prompt: request.prompt,
          autoYes: request.autoYes,
          existingWorktree: request.existingWorktree,
        });

        // Add to local state
        if (response.session) {
          setSessions((prev) => [...prev, response.session!]);
        }

        return response.session ?? null;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to create session"));
        return null;
      }
    },
    []
  );

  // Update session
  const updateSession = useCallback(
    async (
      id: string,
      updates: Partial<UpdateSessionRequest>
    ): Promise<Session | null> => {
      if (!clientRef.current) return null;

      setError(null);

      try {
        const response = await clientRef.current.updateSession({
          id,
          status: updates.status,
          category: updates.category,
          title: updates.title,
        });

        // Update local state
        if (response.session) {
          setSessions((prev) =>
            prev.map((s) => (s.id === id ? response.session! : s))
          );
        }

        return response.session ?? null;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to update session"));
        return null;
      }
    },
    []
  );

  // Delete session
  const deleteSession = useCallback(
    async (id: string, force: boolean = false): Promise<boolean> => {
      if (!clientRef.current) return false;

      setError(null);

      try {
        const response = await clientRef.current.deleteSession({ id, force });

        // Remove from local state
        if (response.success) {
          setSessions((prev) => prev.filter((s) => s.id !== id));
        }

        return response.success;
      } catch (err) {
        setError(err instanceof Error ? err : new Error("Failed to delete session"));
        return false;
      }
    },
    []
  );

  // Pause session
  const pauseSession = useCallback(
    async (id: string): Promise<Session | null> => {
      return updateSession(id, {
        status: SessionStatus.SESSION_STATUS_PAUSED,
      });
    },
    [updateSession]
  );

  // Resume session
  const resumeSession = useCallback(
    async (id: string): Promise<Session | null> => {
      return updateSession(id, {
        status: SessionStatus.SESSION_STATUS_RUNNING,
      });
    },
    [updateSession]
  );

  // Watch sessions for real-time updates
  const watchSessions = useCallback(
    (watchOptions?: { categoryFilter?: string; statusFilter?: SessionStatus }) => {
      if (!clientRef.current) return;

      // Stop any existing watch
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      abortControllerRef.current = new AbortController();

      (async () => {
        try {
          const stream = clientRef.current!.watchSessions(
            {
              categoryFilter: watchOptions?.categoryFilter,
              statusFilter: watchOptions?.statusFilter,
            },
            { signal: abortControllerRef.current!.signal }
          );

          for await (const event of stream) {
            handleSessionEvent(event);
          }
        } catch (err) {
          // Ignore abort errors
          if (err instanceof Error && err.name !== "AbortError") {
            setError(err);
          }
        }
      })();
    },
    []
  );

  // Stop watching sessions
  const stopWatching = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
  }, []);

  // Handle session events from watch stream
  const handleSessionEvent = useCallback((event: SessionEvent) => {
    if (!event.session) return;

    switch (event.eventType) {
      case "created":
        setSessions((prev) => {
          // Avoid duplicates
          if (prev.some((s) => s.id === event.session!.id)) {
            return prev;
          }
          return [...prev, event.session!];
        });
        break;

      case "updated":
        setSessions((prev) =>
          prev.map((s) => (s.id === event.session!.id ? event.session! : s))
        );
        break;

      case "deleted":
        setSessions((prev) => prev.filter((s) => s.id !== event.sessionId));
        break;
    }
  }, []);

  // Auto-watch on mount if enabled
  useEffect(() => {
    if (autoWatch) {
      watchSessions();
    }

    return () => {
      stopWatching();
    };
  }, [autoWatch, watchSessions, stopWatching]);

  // Initial load
  useEffect(() => {
    listSessions();
  }, [listSessions]);

  return {
    sessions,
    loading,
    error,
    listSessions,
    getSession,
    createSession,
    updateSession,
    deleteSession,
    pauseSession,
    resumeSession,
    watchSessions,
    stopWatching,
  };
}
