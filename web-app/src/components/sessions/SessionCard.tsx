"use client";

import { useState } from "react";
import { Session, SessionStatus, ReviewItem } from "@/gen/session/v1/types_pb";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { TagEditor } from "./TagEditor";
import styles from "./SessionCard.module.css";

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  onDelete?: () => void;
  onPause?: () => void;
  onResume?: () => void;
  onDuplicate?: () => void;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  selectMode?: boolean;
  isSelected?: boolean;
  onToggleSelect?: () => void;
  reviewItem?: ReviewItem; // Optional review queue item if session needs attention
}

export function SessionCard({
  session,
  onClick,
  onDelete,
  onPause,
  onResume,
  onDuplicate,
  onUpdateTags,
  selectMode = false,
  isSelected = false,
  onToggleSelect,
  reviewItem,
}: SessionCardProps) {
  const [showTagEditor, setShowTagEditor] = useState(false);
  const getStatusColor = (status: SessionStatus): string => {
    switch (status) {
      case SessionStatus.RUNNING:
        return styles.statusRunning;
      case SessionStatus.READY:
        return styles.statusReady;
      case SessionStatus.PAUSED:
        return styles.statusPaused;
      case SessionStatus.LOADING:
        return styles.statusLoading;
      case SessionStatus.NEEDS_APPROVAL:
        return styles.statusNeedsApproval;
      default:
        return styles.statusUnknown;
    }
  };

  const getStatusText = (status: SessionStatus): string => {
    switch (status) {
      case SessionStatus.RUNNING:
        return "Running";
      case SessionStatus.READY:
        return "Ready";
      case SessionStatus.PAUSED:
        return "Paused";
      case SessionStatus.LOADING:
        return "Loading";
      case SessionStatus.NEEDS_APPROVAL:
        return "Needs Approval";
      default:
        return "Unknown";
    }
  };

  const formatDate = (timestamp?: { seconds: bigint; nanos: number }): string => {
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
  };

  const formatTimeAgo = (timestamp?: { seconds: bigint; nanos: number }): string => {
    if (!timestamp || timestamp.seconds === BigInt(0)) return "Never";
    const now = Date.now();
    const date = new Date(Number(timestamp.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);

    if (seconds < 60) return `${seconds}s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
  };

  const isPaused = session.status === SessionStatus.PAUSED;

  const handleCardClick = (e: React.MouseEvent) => {
    if (selectMode && onToggleSelect) {
      e.stopPropagation();
      onToggleSelect();
    } else if (onClick) {
      onClick();
    }
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onToggleSelect) {
      onToggleSelect();
    }
  };

  const handleEditTags = (e: React.MouseEvent) => {
    e.stopPropagation();
    setShowTagEditor(true);
  };

  const handleSaveTags = (newTags: string[]) => {
    if (onUpdateTags) {
      onUpdateTags(session.id, newTags);
    }
    setShowTagEditor(false);
  };

  const handleCancelTagEdit = () => {
    setShowTagEditor(false);
  };

  return (
    <>
      {showTagEditor && (
        <TagEditor
          tags={session.tags || []}
          onSave={handleSaveTags}
          onCancel={handleCancelTagEdit}
          sessionTitle={session.title}
        />
      )}
    <div
      className={`${styles.card} ${selectMode ? styles.selectMode : ""} ${isSelected ? styles.selected : ""}`}
      onClick={handleCardClick}
    >
      {selectMode && (
        <div className={styles.checkbox} onClick={handleCheckboxClick}>
          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => {}} // Controlled by onClick
            aria-label={`Select ${session.title}`}
          />
        </div>
      )}
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h3 className={styles.title}>{session.title}</h3>
          <div className={styles.badges}>
            {reviewItem && (
              <ReviewQueueBadge
                priority={reviewItem.priority}
                reason={reviewItem.reason}
                compact={true}
              />
            )}
            <span className={`${styles.status} ${getStatusColor(session.status)}`}>
              {getStatusText(session.status)}
            </span>
          </div>
        </div>
        {session.category && (
          <span className={styles.category}>{session.category}</span>
        )}
        <div className={styles.tagsContainer}>
          {session.tags && session.tags.length > 0 && (
            <div className={styles.tags}>
              {session.tags.map((tag, index) => (
                <span key={index} className={styles.tag}>
                  {tag}
                </span>
              ))}
            </div>
          )}
          <button
            className={styles.editTagsButton}
            onClick={handleEditTags}
            title="Edit tags"
          >
            {session.tags && session.tags.length > 0 ? "Edit Tags" : "Add Tags"}
          </button>
        </div>
        {reviewItem && !selectMode && (
          <div className={styles.reviewInfo}>
            <ReviewQueueBadge
              priority={reviewItem.priority}
              reason={reviewItem.reason}
              compact={false}
            />
            {reviewItem.context && (
              <span className={styles.reviewContext}>{reviewItem.context}</span>
            )}
          </div>
        )}
      </div>

      <div className={styles.body}>
        <div className={styles.info}>
          <div className={styles.infoRow}>
            <span className={styles.label}>Program:</span>
            <span className={styles.value}>{session.program}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.label}>Branch:</span>
            <span className={styles.value}>{session.branch}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.label}>Path:</span>
            <span className={styles.value} title={session.path}>
              {session.path}
            </span>
          </div>
          {session.workingDir && (
            <div className={styles.infoRow}>
              <span className={styles.label}>Working Dir:</span>
              <span className={styles.value}>{session.workingDir}</span>
            </div>
          )}
        </div>

        {session.diffStats && (
          <div className={styles.diffStats}>
            <span className={styles.diffAdded}>+{session.diffStats.added}</span>
            <span className={styles.diffRemoved}>-{session.diffStats.removed}</span>
          </div>
        )}
      </div>

      <div className={styles.footer}>
        <div className={styles.timestamps}>
          <span className={styles.timestamp}>
            Created: {formatDate(session.createdAt)}
          </span>
          <span className={styles.timestamp}>
            Updated: {formatDate(session.updatedAt)}
          </span>
          {session.lastMeaningfulOutput && (
            <span className={styles.timestamp} title="Last meaningful terminal output (excluding tmux banners)">
              Last Activity: {formatTimeAgo(session.lastMeaningfulOutput)}
            </span>
          )}
        </div>

        <div className={styles.actions}>
          {isPaused ? (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResume?.();
              }}
            >
              Resume
            </button>
          ) : (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onPause?.();
              }}
            >
              Pause
            </button>
          )}
          <button
            className={styles.actionButton}
            onClick={(e) => {
              e.stopPropagation();
              onDuplicate?.();
            }}
            title="Duplicate this session with editable configuration"
          >
            Duplicate
          </button>
          <button
            className={`${styles.actionButton} ${styles.deleteButton}`}
            onClick={(e) => {
              e.stopPropagation();
              onDelete?.();
            }}
          >
            Delete
          </button>
        </div>
      </div>
    </div>
    </>
  );
}
