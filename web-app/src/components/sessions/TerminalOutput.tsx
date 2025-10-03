"use client";

import { useEffect, useRef, useState } from "react";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
}

export function TerminalOutput({ sessionId, baseUrl }: TerminalOutputProps) {
  const [output, setOutput] = useState<string>("");
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const terminalRef = useRef<HTMLPreElement>(null);
  const autoScrollRef = useRef(true);

  // Auto-scroll to bottom when new output arrives
  useEffect(() => {
    if (terminalRef.current && autoScrollRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [output]);

  // Detect if user scrolls up (disable auto-scroll)
  const handleScroll = () => {
    if (!terminalRef.current) return;

    const { scrollTop, scrollHeight, clientHeight } = terminalRef.current;
    const isAtBottom = Math.abs(scrollHeight - clientHeight - scrollTop) < 10;
    autoScrollRef.current = isAtBottom;
  };

  // Placeholder for streaming terminal output
  useEffect(() => {
    setIsConnected(true);
    setOutput(
      "Terminal output streaming coming soon...\n\n" +
      "This will display real-time terminal output from the session.\n" +
      "Features:\n" +
      "  • Real-time streaming via Server-Sent Events (SSE)\n" +
      "  • ANSI color code support\n" +
      "  • Auto-scroll with manual override\n" +
      "  • Copy output to clipboard\n" +
      "  • Search within output\n" +
      "  • Export to file\n\n" +
      `Session ID: ${sessionId}\n` +
      `Connected to: ${baseUrl}`
    );

    // Cleanup
    return () => {
      setIsConnected(false);
    };
  }, [sessionId, baseUrl]);

  const handleCopyOutput = () => {
    navigator.clipboard.writeText(output);
  };

  const handleClearOutput = () => {
    setOutput("");
  };

  const handleScrollToBottom = () => {
    autoScrollRef.current = true;
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  };

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <div className={styles.status}>
          <span
            className={`${styles.statusIndicator} ${isConnected ? styles.connected : styles.disconnected}`}
          />
          <span className={styles.statusText}>
            {isConnected ? "Connected" : "Disconnected"}
          </span>
        </div>
        <div className={styles.actions}>
          <button
            className={styles.toolbarButton}
            onClick={handleScrollToBottom}
            title="Scroll to bottom"
          >
            ↓ Bottom
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleCopyOutput}
            title="Copy output"
          >
            📋 Copy
          </button>
          <button
            className={styles.toolbarButton}
            onClick={handleClearOutput}
            title="Clear output"
          >
            🗑️ Clear
          </button>
        </div>
      </div>
      {error && <div className={styles.error}>{error}</div>}
      <pre
        ref={terminalRef}
        className={styles.terminal}
        onScroll={handleScroll}
      >
        {output || "No output yet..."}
      </pre>
    </div>
  );
}
