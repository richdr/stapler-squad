"use client";

import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { formatTimeAgo, truncateMiddle } from "@/lib/utils/timestamp";
import styles from "./HistoryEntryCard.module.css";

interface HistoryEntryCardProps {
  entry: ClaudeHistoryEntry;
  isSelected: boolean;
  onSelect: () => void;
}

export function HistoryEntryCard({ entry, isSelected, onSelect }: HistoryEntryCardProps) {
  return (
    <div
      onClick={onSelect}
      className={`${styles.entryCard} ${isSelected ? styles.selected : ""}`}
      role="button"
      tabIndex={0}
      aria-selected={isSelected}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect();
        }
      }}
    >
      <div className={styles.entryHeader}>
        <div className={styles.entryName}>{entry.name}</div>
        <div className={styles.entryTime}>{formatTimeAgo(entry.updatedAt)}</div>
      </div>
      <div className={styles.entryMeta}>
        <span className={styles.entryModel}>{entry.model}</span>
        <span className={styles.entryDivider}>•</span>
        <span className={styles.entryMessages}>
          {entry.messageCount} {entry.messageCount === 1 ? "message" : "messages"}
        </span>
      </div>
      {entry.project && (
        <div className={styles.entryProject} title={entry.project}>
          📁 {truncateMiddle(entry.project, 50)}
        </div>
      )}
    </div>
  );
}
