"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import { XtermTerminal } from "./XtermTerminal";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
}

export function TerminalOutput({ sessionId, baseUrl }: TerminalOutputProps) {
  const xtermRef = useRef<any>(null);
  const [connectionAttempts, setConnectionAttempts] = useState(0);
  const [showReconnectButton, setShowReconnectButton] = useState(false);
  const reconnectTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const previousConnectionStateRef = useRef(false);
  const lastResizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const refreshCountRef = useRef(0); // Track number of forced refreshes

  // Scrollback loading state
  const [isLoadingHistory, setIsLoadingHistory] = useState(false);
  const [hasMoreHistory, setHasMoreHistory] = useState(true);
  const [oldestLoadedSequence, setOldestLoadedSequence] = useState<number>(0);
  const scrollPositionBeforeLoadRef = useRef<number>(0);
  const isScrollingToTopRef = useRef(false);

  // Debug mode state - synced with localStorage
  const [debugMode, setDebugMode] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("debug-terminal") === "true";
    }
    return false;
  });

  // Callback to write scrollback directly to terminal
  // For historical scrollback, we prepend at the top and maintain scroll position
  const handleScrollbackReceived = useCallback((scrollback: string, metadata?: { hasMore: boolean; oldestSequence: number; newestSequence: number; totalLines: number }) => {
    console.log(`[TerminalOutput] Received ${scrollback.length} bytes of scrollback`, metadata);
    if (!xtermRef.current?.terminal) return;

    const terminal = xtermRef.current.terminal;

    // Update metadata state if provided (for historical loads)
    if (metadata) {
      setHasMoreHistory(metadata.hasMore);
      setOldestLoadedSequence(metadata.oldestSequence);
      console.log(`[TerminalOutput] Updated scrollback state: hasMore=${metadata.hasMore}, oldestSeq=${metadata.oldestSequence}`);

      // Historical load - prepend at top and maintain scroll position
      const buffer = terminal.buffer.active;
      const scrollFromBottom = buffer.length - buffer.viewportY;

      // Write historical content
      terminal.write(scrollback);

      // Restore scroll position (maintaining user's view)
      setTimeout(() => {
        const newLines = scrollback.split('\n').length;
        terminal.scrollLines(-newLines);
        setIsLoadingHistory(false);
      }, 10);
    } else {
      // Current pane content - just write and scroll to bottom
      terminal.write(scrollback);
      terminal.scrollToBottom();
    }
  }, []);

  // Callback to write output directly to terminal (bypasses React state for better performance)
  const handleOutput = useCallback((output: string) => {
    if (xtermRef.current) {
      xtermRef.current.write(output);
    }
  }, []);

  // Wrap terminal refresh method to detect all refresh calls
  useEffect(() => {
    if (xtermRef.current?.terminal && !xtermRef.current.terminal._refreshMonitorInstalled) {
      const terminal = xtermRef.current.terminal;
      const originalRefresh = terminal.refresh.bind(terminal);
      const originalWrite = terminal.write.bind(terminal);

      // Track write operations to detect race conditions
      let lastWriteTime = 0;
      let writeCount = 0;

      // Wrap write to track output timing
      terminal.write = (data: string | Uint8Array, callback?: () => void) => {
        const now = performance.now();
        const timeSinceLastWrite = now - lastWriteTime;
        lastWriteTime = now;
        writeCount++;

        // Only log if verbose debugging is enabled
        if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true") {
          console.log('[XtermWrite]', {
            writeCount,
            dataLength: data.length,
            timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
            cursorY: terminal.buffer.active.cursorY,
            timestamp: new Date().toISOString()
          });
        }

        return originalWrite(data, callback);
      };

      // Wrap refresh to log all calls and detect race conditions
      terminal.refresh = (start: number, end: number) => {
        const stackTrace = new Error().stack;
        const caller = stackTrace?.split('\n')[2]?.trim() || 'unknown';
        const timeSinceLastWrite = performance.now() - lastWriteTime;

        console.log('[XtermRefresh] Refresh called', {
          start,
          end,
          rows: terminal.rows,
          timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
          recentWrites: writeCount,
          caller: caller.replace(/^at /, ''),
          possibleRaceCondition: timeSinceLastWrite < 50, // Flag if refresh happens <50ms after write
          timestamp: new Date().toISOString()
        });

        // Reset write counter after refresh
        writeCount = 0;

        return originalRefresh(start, end);
      };

      terminal._refreshMonitorInstalled = true;
      console.log('[TerminalOutput] Refresh and write monitoring installed');
    }
  }, [xtermRef.current?.terminal]);

  const { isConnected, error, sendInput, resize, connect, disconnect, scrollbackLoaded, requestScrollback } = useTerminalStream({
    baseUrl,
    sessionId,
    terminal: xtermRef.current?.terminal || null, // Pass terminal for delta compression
    scrollbackLines: 1000,
    onError: (err) => {
      console.error("Terminal stream error:", err);
      setConnectionAttempts((prev) => prev + 1);
    },
    onScrollbackReceived: handleScrollbackReceived,
    onOutput: handleOutput,
  });

  // Disconnect WebSocket on unmount
  useEffect(() => {
    return () => {
      disconnect();
    };
  }, [disconnect]);

  // Handle terminal data input
  const handleTerminalData = useCallback((data: string) => {
    sendInput(data);
  }, [sendInput]);

  // Handle terminal resize - only send if size actually changed
  const handleTerminalResize = useCallback((cols: number, rows: number) => {
    if (!isConnected) {
      console.log(`[TerminalOutput] Resize blocked - not connected (${cols}x${rows})`);
      return;
    }

    // Block resize messages during scrollback load to prevent feedback loop
    if (!scrollbackLoaded) {
      console.log(`[TerminalOutput] Resize blocked - scrollback loading (${cols}x${rows})`);
      // Save the size for after scrollback completes
      lastResizeRef.current = { cols, rows };
      return;
    }

    const lastResize = lastResizeRef.current;
    if (lastResize && lastResize.cols === cols && lastResize.rows === rows) {
      // Size unchanged, don't send resize message
      console.log(`[TerminalOutput] Resize blocked - unchanged (${cols}x${rows})`);
      return;
    }

    console.log(`[TerminalOutput] Sending resize: ${cols}x${rows} (prev: ${lastResize?.cols || 'none'}x${lastResize?.rows || 'none'})`);
    lastResizeRef.current = { cols, rows };
    resize(cols, rows);
  }, [isConnected, scrollbackLoaded, resize]);

  // Monitor connection state changes and show notifications
  useEffect(() => {
    const wasConnected = previousConnectionStateRef.current;
    previousConnectionStateRef.current = isConnected;

    if (!wasConnected && isConnected) {
      // Just connected
      console.log("[TerminalOutput] Connection established");
      setShowReconnectButton(false);
      setConnectionAttempts(0);

      // Clear any pending reconnect timeout
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
    } else if (wasConnected && !isConnected) {
      // Just disconnected
      console.log("[TerminalOutput] Connection lost, will attempt reconnection");

      // Show reconnect button after 5 seconds if still disconnected
      reconnectTimeoutRef.current = setTimeout(() => {
        if (!isConnected) {
          setShowReconnectButton(true);
        }
      }, 5000);
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [isConnected]);

  // Auto-reconnect with exponential backoff
  useEffect(() => {
    if (!isConnected && error && connectionAttempts > 0 && connectionAttempts < 5) {
      const backoffDelay = Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000);
      console.log(`[TerminalOutput] Auto-reconnecting in ${backoffDelay}ms (attempt ${connectionAttempts})`);

      const timeout = setTimeout(() => {
        console.log("[TerminalOutput] Attempting reconnection...");
        connect();
      }, backoffDelay);

      return () => clearTimeout(timeout);
    }
  }, [isConnected, error, connectionAttempts, connect]);

  // Send resize after scrollback load completes
  useEffect(() => {
    if (scrollbackLoaded && isConnected) {
      // If we have a saved resize from during scrollback load, use it
      if (lastResizeRef.current) {
        const { cols, rows } = lastResizeRef.current;
        console.log(`[TerminalOutput] Scrollback complete, sending saved resize: ${cols}x${rows}`);
        resize(cols, rows);
      } else if (xtermRef.current?.terminal) {
        // Otherwise, fit terminal and send current size
        console.log("[TerminalOutput] Scrollback complete, fitting terminal and sending resize");
        xtermRef.current.fit();
        const terminal = xtermRef.current.terminal;
        const cols = terminal.cols;
        const rows = terminal.rows;
        console.log(`[TerminalOutput] Initial resize after scrollback: ${cols}x${rows}`);
        lastResizeRef.current = { cols, rows };
        resize(cols, rows);
      }
    }
  }, [scrollbackLoaded, isConnected, resize]);

  const handleManualReconnect = useCallback(() => {
    console.log("[TerminalOutput] Manual reconnect requested");
    setConnectionAttempts(0);
    setShowReconnectButton(false);
    connect();
  }, [connect]);

  // Toggle debug mode
  const handleToggleDebug = useCallback(() => {
    const newDebugMode = !debugMode;
    setDebugMode(newDebugMode);

    // Sync with localStorage
    if (typeof window !== "undefined") {
      if (newDebugMode) {
        localStorage.setItem("debug-terminal", "true");
        console.log("%c[TerminalOutput] Debug mode ENABLED", "color: #00ff00; font-weight: bold");
        console.log("All terminal refresh and write operations will be logged");
      } else {
        localStorage.removeItem("debug-terminal");
        console.log("%c[TerminalOutput] Debug mode DISABLED", "color: #ff0000; font-weight: bold");
      }
    }
  }, [debugMode]);

  const handleCopyOutput = () => {
    // XtermTerminal handles copy internally via browser selection
    document.execCommand('copy');
  };

  const handleScrollToBottom = () => {
    if (xtermRef.current?.terminal) {
      xtermRef.current.terminal.scrollToBottom();
    }
  };

  const handleClear = () => {
    if (xtermRef.current && xtermRef.current.terminal) {
      const terminal = xtermRef.current.terminal;
      const startTime = performance.now();
      refreshCountRef.current++;

      // Log clear operation start
      console.log('[TerminalOutput] Clear requested', {
        refreshCount: refreshCountRef.current,
        bufferSize: terminal.buffer.active.length,
        rows: terminal.rows,
        cols: terminal.cols,
        scrollbackSize: terminal.buffer.normal.length
      });

      // Clear the terminal buffer
      xtermRef.current.clear();
      const clearTime = performance.now();

      // Force a full screen refresh to prevent corrupted output
      // This ensures xterm.js redraws the entire viewport properly
      terminal.refresh(0, terminal.rows - 1);
      const refreshTime = performance.now();

      // Additionally, reset the cursor to home position for clean state
      terminal.write('\x1b[H');
      const cursorResetTime = performance.now();

      // Log performance metrics and refresh details
      console.log('[TerminalOutput] Clear completed with forced refresh', {
        refreshCount: refreshCountRef.current,
        clearDuration: `${(clearTime - startTime).toFixed(2)}ms`,
        refreshDuration: `${(refreshTime - clearTime).toFixed(2)}ms`,
        cursorResetDuration: `${(cursorResetTime - refreshTime).toFixed(2)}ms`,
        totalDuration: `${(cursorResetTime - startTime).toFixed(2)}ms`,
        refreshedRows: `0-${terminal.rows - 1}`,
        viewport: {
          rows: terminal.rows,
          cols: terminal.cols,
          scrollTop: terminal.buffer.active.viewportY
        }
      });
    }
  };

  const handleManualResize = () => {
    console.log("[TerminalOutput] Manual resize triggered");
    if (xtermRef.current) {
      // Call fit() to resize terminal to container
      xtermRef.current.fit();

      // Get current terminal size after fit
      const terminal = xtermRef.current.terminal;
      if (terminal) {
        const cols = terminal.cols;
        const rows = terminal.rows;
        console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);

        // Force send resize message to backend even if blocked by scrollback
        if (isConnected) {
          console.log(`[TerminalOutput] Forcing resize message to backend: ${cols}x${rows}`);
          lastResizeRef.current = { cols, rows };
          resize(cols, rows);
        }
      }
    }
  };

  const handleLoadMoreHistory = useCallback(() => {
    if (isLoadingHistory || !hasMoreHistory || !isConnected) {
      console.log(`[TerminalOutput] Cannot load history: loading=${isLoadingHistory}, hasMore=${hasMoreHistory}, connected=${isConnected}`);
      return;
    }

    setIsLoadingHistory(true);
    console.log(`[TerminalOutput] Loading more history from sequence ${oldestLoadedSequence}`);

    // Request 200 more lines of history
    requestScrollback(oldestLoadedSequence, 200);
  }, [isLoadingHistory, hasMoreHistory, isConnected, oldestLoadedSequence, requestScrollback]);

  // Infinite scroll detection - load more when scrolled to top
  useEffect(() => {
    const terminal = xtermRef.current?.terminal;
    if (!terminal || !isConnected) return;

    const handleScroll = () => {
      const buffer = terminal.buffer.active;
      const isAtTop = buffer.viewportY === 0;

      if (isAtTop && !isScrollingToTopRef.current && !isLoadingHistory && hasMoreHistory) {
        console.log('[TerminalOutput] Scrolled to top, triggering auto-load');
        isScrollingToTopRef.current = true;
        handleLoadMoreHistory();

        // Reset flag after a delay
        setTimeout(() => {
          isScrollingToTopRef.current = false;
        }, 1000);
      }
    };

    // Listen for scroll events
    terminal.onScroll(handleScroll);
  }, [isConnected, isLoadingHistory, hasMoreHistory, handleLoadMoreHistory]);

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <div className={styles.status}>
          <span
            className={`${styles.statusIndicator} ${
              isConnected ? styles.connected : styles.disconnected
            }`}
          />
          <span className={styles.statusText}>
            {isConnected ? "Connected" : "Disconnected"}
          </span>
          {isConnected && !scrollbackLoaded && (
            <span className={styles.statusText}> • Loading scrollback...</span>
          )}
          {!isConnected && connectionAttempts > 0 && connectionAttempts < 5 && (
            <span className={styles.statusText}>
              {" "}• Reconnecting (attempt {connectionAttempts}/5)...
            </span>
          )}
          {!isConnected && connectionAttempts >= 5 && (
            <span className={styles.errorText}> • Connection failed</span>
          )}
          {error && !isConnected && (
            <span className={styles.errorText}> • {error.message}</span>
          )}
        </div>
        <div className={styles.actions}>
          {showReconnectButton && (
            <button
              className={styles.toolbarButton}
              onClick={handleManualReconnect}
              title="Reconnect to terminal"
              aria-label="Reconnect to terminal"
            >
              🔄 Reconnect
            </button>
          )}
          <button
            className={styles.toolbarButton}
            onClick={handleLoadMoreHistory}
            disabled={isLoadingHistory || !hasMoreHistory || !isConnected}
            title={!hasMoreHistory ? "No more history available" : isLoadingHistory ? "Loading..." : "Load more history (scroll to top for auto-load)"}
            aria-label="Load more terminal history"
            style={isLoadingHistory ? { opacity: 0.6, cursor: 'wait' } : !hasMoreHistory ? { opacity: 0.4, cursor: 'not-allowed' } : {}}
          >
            {isLoadingHistory ? '⏳ Loading...' : hasMoreHistory ? '📜 Load History' : '📜 No More'}
          </button>
          <button
            className={`${styles.toolbarButton} ${debugMode ? styles.debugActive : ''}`}
            onClick={handleToggleDebug}
            title={debugMode ? "Disable debug logging" : "Enable debug logging"}
            aria-label={debugMode ? "Disable debug mode" : "Enable debug mode"}
            style={debugMode ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
          >
            🛠️ {debugMode ? 'Debug ON' : 'Debug'}
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleManualResize}
            title="Resize terminal to fit container"
            aria-label="Resize terminal"
          >
            ↔️ Resize
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleClear}
            title="Clear terminal"
            aria-label="Clear terminal"
          >
            🗑️ Clear
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleScrollToBottom}
            title="Scroll to bottom"
            aria-label="Scroll to bottom"
          >
            ↓ Bottom
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleCopyOutput}
            title="Copy output"
            aria-label="Copy terminal output to clipboard"
          >
            📋 Copy
          </button>
        </div>
      </div>
      <div className={styles.terminal}>
        <XtermTerminal
          ref={xtermRef}
          onData={handleTerminalData}
          onResize={handleTerminalResize}
          theme="dark"
          fontSize={14}
          scrollback={10000}
        />
      </div>
    </div>
  );
}
