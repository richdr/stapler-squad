"use client";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { TerminalData, TerminalInput, TerminalResize } from "@/gen/session/v1/events_pb";
import { useEffect, useRef, useState, useCallback } from "react";

interface UseTerminalStreamOptions {
  baseUrl: string;
  sessionId: string;
  onError?: (error: Error) => void;
}

interface TerminalStreamResult {
  output: string;
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number) => void;
  connect: () => void;
  disconnect: () => void;
}

// Queue to manage outgoing terminal messages
class MessageQueue {
  private queue: TerminalData[] = [];
  private resolve: ((value: TerminalData) => void) | null = null;
  private closed = false;

  push(msg: TerminalData) {
    if (this.closed) return;

    if (this.resolve) {
      this.resolve(msg);
      this.resolve = null;
    } else {
      this.queue.push(msg);
    }
  }

  async *[Symbol.asyncIterator]() {
    while (!this.closed || this.queue.length > 0) {
      if (this.queue.length > 0) {
        yield this.queue.shift()!;
      } else {
        const msg = await new Promise<TerminalData>((resolve) => {
          this.resolve = resolve;
        });
        yield msg;
      }
    }
  }

  close() {
    this.closed = true;
    if (this.resolve) {
      // Force unblock the iterator
      this.resolve(new TerminalData({ sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
  }
}

export function useTerminalStream({
  baseUrl,
  sessionId,
  onError,
}: UseTerminalStreamOptions): TerminalStreamResult {
  const [output, setOutput] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const clientRef = useRef(createPromiseClient(
    SessionService,
    createConnectTransport({ baseUrl })
  ));

  const connect = useCallback(async () => {
    if (isConnected || !sessionId) return;

    try {
      abortControllerRef.current = new AbortController();
      messageQueueRef.current = new MessageQueue();

      // Send initial handshake message
      messageQueueRef.current.push(
        new TerminalData({
          sessionId,
          data: { case: undefined }, // Initial handshake
        })
      );

      // Create bidirectional stream
      const stream = clientRef.current.streamTerminal(
        messageQueueRef.current,
        { signal: abortControllerRef.current.signal }
      );

      setIsConnected(true);
      setError(null);

      // Start reading terminal output in background
      (async () => {
        try {
          for await (const msg of stream) {
            if (msg.data.case === "output") {
              // Decode bytes to string
              const text = new TextDecoder().decode(msg.data.value.data);
              setOutput((prev) => prev + text);
            } else if (msg.data.case === "error") {
              const err = new Error(msg.data.value.message);
              setError(err);
              onError?.(err);
            }
          }
        } catch (err) {
          const error = err instanceof Error ? err : new Error(String(err));
          setError(error);
          onError?.(error);
        } finally {
          setIsConnected(false);
        }
      })();
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      setError(error);
      onError?.(error);
      setIsConnected(false);
    }
  }, [baseUrl, sessionId, isConnected, onError]);

  const disconnect = useCallback(async () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    if (messageQueueRef.current) {
      messageQueueRef.current.close();
      messageQueueRef.current = null;
    }
    setIsConnected(false);
  }, []);

  const sendInput = useCallback(
    (input: string) => {
      if (!messageQueueRef.current || !isConnected) {
        console.warn("Cannot send input: stream not connected");
        return;
      }

      try {
        const inputBytes = new TextEncoder().encode(input);
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "input",
              value: new TerminalInput({ data: inputBytes }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, isConnected, onError]
  );

  const resize = useCallback(
    (cols: number, rows: number) => {
      if (!messageQueueRef.current || !isConnected) {
        console.warn("Cannot resize terminal: stream not connected");
        return;
      }

      try {
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "resize",
              value: new TerminalResize({ cols, rows }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, isConnected, onError]
  );

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => {
      disconnect();
    };
  }, [sessionId]); // Only reconnect if sessionId changes

  return {
    output,
    isConnected,
    error,
    sendInput,
    resize,
    connect,
    disconnect,
  };
}
