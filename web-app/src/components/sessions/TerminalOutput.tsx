"use client";

import { useEffect, useRef } from "react";
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";
import Convert from "ansi-to-html";
import styles from "./TerminalOutput.module.css";

interface TerminalOutputProps {
  sessionId: string;
  baseUrl: string;
}

export function TerminalOutput({ sessionId, baseUrl }: TerminalOutputProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const autoScrollRef = useRef(true);
  const converterRef = useRef(
    new Convert({
      fg: "#d4d4d4",
      bg: "#1e1e1e",
      newline: true,
      escapeXML: true,
      stream: false,
    })
  );

  const { output, isConnected, error } = useTerminalStream({
    baseUrl,
    sessionId,
    onError: (err) => console.error("Terminal stream error:", err),
  });

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

  const handleCopyOutput = () => {
    navigator.clipboard.writeText(output);
  };

  const handleScrollToBottom = () => {
    autoScrollRef.current = true;
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  };

  // Convert ANSI codes to HTML
  const htmlOutput = output ? converterRef.current.toHtml(output) : "";

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
          {error && <span className={styles.errorText}>{error.message}</span>}
        </div>
        <div className={styles.actions}>
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
      <div
        ref={terminalRef}
        className={styles.terminal}
        onScroll={handleScroll}
        role="log"
        aria-live="polite"
        aria-label="Terminal output"
        dangerouslySetInnerHTML={{
          __html: htmlOutput || "<span style='opacity: 0.5'>Waiting for terminal output...</span>",
        }}
      />
    </div>
  );
}
