"use client";

import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";
import { TerminalData, TerminalInput, TerminalResize, ScrollbackRequest, CurrentPaneRequest } from "@/gen/session/v1/events_pb";
import { createWebsocketBasedTransport } from "@/lib/transport/websocket-transport";
import { useEffect, useRef, useState, useCallback } from "react";
import { DeltaApplicator } from "@/lib/terminal/DeltaApplicator";
import type { Terminal } from '@xterm/xterm';

interface ScrollbackMetadata {
  hasMore: boolean;
  oldestSequence: number;
  newestSequence: number;
  totalLines: number;
}

interface UseTerminalStreamOptions {
  baseUrl: string;
  sessionId: string;
  terminal?: Terminal | null; // Terminal instance for delta compression
  scrollbackLines?: number; // Number of lines to request from scrollback
  onError?: (error: Error) => void;
  onScrollbackReceived?: (scrollback: string, metadata?: ScrollbackMetadata) => void; // Callback when scrollback is received
  onOutput?: (output: string) => void; // Callback when new output is received (bypass React state)
}

interface TerminalStreamResult {
  output: string; // Deprecated: Use onOutput callback for better performance
  isConnected: boolean;
  error: Error | null;
  sendInput: (input: string) => void;
  resize: (cols: number, rows: number) => void;
  connect: () => void;
  disconnect: () => void;
  scrollbackLoaded: boolean; // Indicates if scrollback has been loaded
  requestScrollback: (fromSequence: number, limit: number) => void; // Request historical scrollback
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
        // Don't yield sentinel messages (empty messages used to unblock iterator)
        if (msg.sessionId !== "" || msg.data.case !== undefined) {
          yield msg;
        }
      }
    }
  }

  close() {
    this.closed = true;
    if (this.resolve) {
      // Force unblock the iterator with a sentinel message
      // This message will be filtered out by the iterator and not sent to the server
      this.resolve(new TerminalData({ sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
  }
}

export function useTerminalStream({
  baseUrl,
  sessionId,
  terminal,
  scrollbackLines = 1000,
  onError,
  onScrollbackReceived,
  onOutput,
}: UseTerminalStreamOptions): TerminalStreamResult {
  const [output, setOutput] = useState("");
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [scrollbackLoaded, setScrollbackLoaded] = useState(false);

  const messageQueueRef = useRef<MessageQueue | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isDisconnectingRef = useRef(false);
  const isConnectedRef = useRef(false); // Track connection state in ref for callbacks
  const lastResizeTimeRef = useRef<number>(0); // Timestamp of last resize message sent
  const deltaApplicatorRef = useRef<DeltaApplicator | null>(null);
  const clientRef = useRef(createPromiseClient(
    SessionService,
    createWebsocketBasedTransport({
      baseUrl,
      useBinaryFormat: true, // WebSocket supports binary format
    })
  ));

  // Performance optimization: Batch terminal output updates
  const outputBufferRef = useRef<string[]>([]);
  const pendingUpdateRef = useRef<NodeJS.Timeout | null>(null);
  const textDecoderRef = useRef(new TextDecoder()); // Reuse decoder for performance

  // Sync ref with state
  useEffect(() => {
    isConnectedRef.current = isConnected;
  }, [isConnected]);

  // Flush buffered output to state (batched with minimal delay)
  const flushOutputBuffer = useCallback(() => {
    if (outputBufferRef.current.length > 0) {
      const bufferedText = outputBufferRef.current.join("");
      outputBufferRef.current = [];
      setOutput((prev) => prev + bufferedText);
    }
    pendingUpdateRef.current = null;
  }, []);

  // Schedule output update (batched with 5ms delay to reduce flickering while maintaining responsiveness)
  const scheduleOutputUpdate = useCallback((text: string) => {
    outputBufferRef.current.push(text);

    if (!pendingUpdateRef.current) {
      pendingUpdateRef.current = setTimeout(flushOutputBuffer, 5);
    }
  }, [flushOutputBuffer]);

  const connect = useCallback(async () => {
    if (isConnectedRef.current || !sessionId) {
      return;
    }

    // Reset disconnecting flag on connect
    isDisconnectingRef.current = false;

    // Initialize delta applicator if terminal is available
    if (terminal && !deltaApplicatorRef.current) {
      deltaApplicatorRef.current = new DeltaApplicator(terminal);
      console.log('[useTerminalStream] Delta applicator initialized');
    }

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

      // Request current pane content (what user would see if they attached to tmux)
      // This gives us the current terminal state, ideal for apps that rewrite lines
      messageQueueRef.current.push(
        new TerminalData({
          sessionId,
          data: {
            case: "currentPaneRequest",
            value: new CurrentPaneRequest({
              lines: 50, // Get last 50 lines (typical terminal viewport)
              includeEscapes: true, // Preserve colors and formatting
            }),
          },
        })
      );

      // Create bidirectional stream
      const stream = clientRef.current.streamTerminal(
        messageQueueRef.current,
        { signal: abortControllerRef.current.signal }
      );

      setError(null);

      // Start reading terminal output in background
      (async () => {
        try {
          let firstMessage = true;
          for await (const msg of stream) {
            // Set connected after receiving first message
            if (firstMessage) {
              setIsConnected(true);
              firstMessage = false;
            }

            if (msg.data.case === "delta") {
              // Handle delta compression
              if (deltaApplicatorRef.current) {
                const success = deltaApplicatorRef.current.applyDelta(msg.data.value);
                if (!success) {
                  // Desync detected - request full sync by resetting version
                  console.error('[useTerminalStream] Delta desync detected, terminal may be out of sync');
                  deltaApplicatorRef.current.resetVersion();
                  // TODO: Send error message to server to request full sync
                }
              } else {
                console.warn('[useTerminalStream] Received delta but no delta applicator initialized');
              }
            } else if (msg.data.case === "output") {
              // Handle raw output (fallback mode or when delta not available)
              const text = textDecoderRef.current.decode(msg.data.value.data, { stream: true });
              // Only log if debug mode is enabled (toggle with: localStorage.setItem('debug-terminal', 'true'))
              if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
                console.debug(`[useTerminalStream] Received output: ${text.length} bytes`);
              }

              // Use callback if provided (better performance - no React state updates)
              if (onOutput) {
                onOutput(text);
              } else {
                // Fallback: Use batched updates for backward compatibility
                scheduleOutputUpdate(text);
              }
            } else if (msg.data.case === "currentPaneResponse") {
              // Handle current pane content (what tmux shows now)
              const response = msg.data.value;
              const content = textDecoderRef.current.decode(response.content);

              console.log(`[useTerminalStream] Received current pane: ${content.length} bytes, ` +
                          `cursor at (${response.cursorX},${response.cursorY}), ` +
                          `pane size: ${response.paneWidth}x${response.paneHeight}`);

              // Write current pane content to terminal
              if (onScrollbackReceived) {
                onScrollbackReceived(content);
              }

              // Mark as loaded (reuse scrollbackLoaded flag for compatibility)
              setScrollbackLoaded(true);
            } else if (msg.data.case === "scrollbackResponse") {
              // Keep scrollback support for "load more history" feature
              // Optimize: Use array and join instead of concatenation
              const chunks: string[] = [];
              for (const chunk of msg.data.value.chunks) {
                const text = textDecoderRef.current.decode(chunk.data);
                chunks.push(text);
              }
              const scrollbackText = chunks.join("");

              // Extract metadata for smart caching and UI state
              const metadata: ScrollbackMetadata = {
                hasMore: msg.data.value.hasMore,
                oldestSequence: Number(msg.data.value.oldestSequence),
                newestSequence: Number(msg.data.value.newestSequence),
                totalLines: Number(msg.data.value.totalLines),
              };

              console.log(`[useTerminalStream] Scrollback metadata:`, metadata);

              // Call callback to write directly to terminal with metadata
              if (onScrollbackReceived) {
                onScrollbackReceived(scrollbackText, metadata);
              }

              setScrollbackLoaded(true);
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
  }, [sessionId, terminal, scrollbackLines, onError, onScrollbackReceived, onOutput, scheduleOutputUpdate]); // Include terminal and onOutput

  const disconnect = useCallback(async () => {
    // Prevent double-disconnect
    if (isDisconnectingRef.current) {
      return;
    }
    isDisconnectingRef.current = true;

    // Graceful shutdown: close message queue first to stop sending
    // This allows the server to send EndStreamResponse before closing
    if (messageQueueRef.current) {
      messageQueueRef.current.close();
      messageQueueRef.current = null;
    }

    // Give the stream time to close gracefully (wait for EndStreamResponse)
    // Use Promise-based waiting without polling to avoid setInterval violations
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(() => {
        if (abortControllerRef.current) {
          console.debug("[useTerminalStream] Timeout waiting for graceful close, forcing abort");
          abortControllerRef.current.abort();
          abortControllerRef.current = null;
        }
        resolve();
      }, 1000); // 1 second timeout for graceful close

      // Use event-driven approach instead of polling
      // Check immediately if already disconnected
      if (!isConnectedRef.current) {
        clearTimeout(timeout);
        resolve();
        return;
      }

      // Otherwise wait for timeout - the connection state change will trigger cleanup
      // This avoids the 100ms polling interval that causes performance violations
    });

    setIsConnected(false);
    isDisconnectingRef.current = false;
  }, []); // No dependencies - use refs for all state checks

  const sendInput = useCallback(
    (input: string) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
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
    [sessionId, onError]
  );

  const resize = useCallback(
    (cols: number, rows: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot resize terminal: stream not connected");
        return;
      }

      // Throttle resize messages to max 1 per second to prevent feedback loops
      const now = Date.now();
      const timeSinceLastResize = now - lastResizeTimeRef.current;
      const THROTTLE_MS = 1000; // 1 second throttle

      if (timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
        console.log(`[useTerminalStream] Resize throttled (${timeSinceLastResize}ms since last, need ${THROTTLE_MS}ms)`);
        return;
      }

      try {
        console.log(`[useTerminalStream] Pushing resize message to queue: ${cols}x${rows}`);
        lastResizeTimeRef.current = now;
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
    [sessionId, onError]
  );

  const requestScrollback = useCallback(
    (fromSequence: number, limit: number) => {
      if (!messageQueueRef.current || !isConnectedRef.current) {
        console.warn("Cannot request scrollback: stream not connected");
        return;
      }

      try {
        console.log(`[useTerminalStream] Requesting scrollback: fromSeq=${fromSequence}, limit=${limit}`);
        messageQueueRef.current.push(
          new TerminalData({
            sessionId,
            data: {
              case: "scrollbackRequest",
              value: new ScrollbackRequest({
                fromSequence: BigInt(fromSequence),
                limit,
              }),
            },
          })
        );
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
      }
    },
    [sessionId, onError]
  );

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => {
      // Cleanup: Cancel any pending timeout updates
      if (pendingUpdateRef.current) {
        clearTimeout(pendingUpdateRef.current);
        pendingUpdateRef.current = null;
      }
      // Flush any remaining buffered output
      flushOutputBuffer();
      disconnect();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]); // Only reconnect if sessionId changes

  return {
    output,
    isConnected,
    error,
    sendInput,
    resize,
    connect,
    disconnect,
    scrollbackLoaded,
    requestScrollback,
  };
}
