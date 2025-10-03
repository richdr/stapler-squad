"use client";

import styles from "./BulkActions.module.css";

interface BulkActionsProps {
  selectedCount: number;
  onPauseAll: () => void;
  onResumeAll: () => void;
  onDeleteAll: () => void;
  onSelectAll: () => void;
  onClearSelection: () => void;
  totalCount: number;
}

export function BulkActions({
  selectedCount,
  onPauseAll,
  onResumeAll,
  onDeleteAll,
  onSelectAll,
  onClearSelection,
  totalCount,
}: BulkActionsProps) {
  return (
    <div className={styles.container}>
      <div className={styles.selection}>
        <span className={styles.count}>
          {selectedCount} of {totalCount} selected
        </span>
        {selectedCount < totalCount && (
          <button onClick={onSelectAll} className={styles.selectAllButton}>
            Select All
          </button>
        )}
        {selectedCount > 0 && (
          <button onClick={onClearSelection} className={styles.clearButton}>
            Clear Selection
          </button>
        )}
      </div>

      <div className={styles.actions}>
        <button
          onClick={onPauseAll}
          className={styles.actionButton}
          disabled={selectedCount === 0}
        >
          ⏸️ Pause Selected
        </button>
        <button
          onClick={onResumeAll}
          className={styles.actionButton}
          disabled={selectedCount === 0}
        >
          ▶️ Resume Selected
        </button>
        <button
          onClick={onDeleteAll}
          className={`${styles.actionButton} ${styles.danger}`}
          disabled={selectedCount === 0}
        >
          🗑️ Delete Selected
        </button>
      </div>
    </div>
  );
}
