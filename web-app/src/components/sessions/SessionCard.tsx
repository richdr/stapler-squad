"use client";

import { useState, useEffect } from "react";
import { Session, SessionStatus, ReviewItem, InstanceType, LookoutState } from "@/gen/session/v1/types_pb";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { GitHubBadge } from "./GitHubBadge";
import { TagEditor } from "./TagEditor";
import styles from "./SessionCard.module.css";

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  onDelete?: () => Promise<void> | void;
  onPause?: () => void;
  onResume?: () => void;
  onDuplicate?: () => void;
  onRename?: (sessionId: string, newTitle: string) => Promise<boolean>;
  onRestart?: (sessionId: string) => Promise<boolean>;
  onUpdateTags?: (sessionId: string, tags: string[]) => void;
  onToggleGoingDark?: (sessionId: string, enabled: boolean) => Promise<void> | void;
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
  onRename,
  onRestart,
  onUpdateTags,
  onToggleGoingDark,
  selectMode = false,
  isSelected = false,
  onToggleSelect,
  reviewItem,
}: SessionCardProps) {
  const [isTagEditorOpen, setIsTagEditorOpen] = useState(false);
  const [isRenameOpen, setIsRenameOpen] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [newTitle, setNewTitle] = useState(session.title);
  const [isRestartConfirmOpen, setIsRestartConfirmOpen] = useState(false);
  const [isRenaming, setIsRenaming] = useState(false);
  const [isRestarting, setIsRestarting] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [renameError, setRenameError] = useState("");
  const [isTogglingDark, setIsTogglingDark] = useState(false);
  const [toggleDarkError, setToggleDarkError] = useState("");
  const [isGoingDarkConfirmOpen, setIsGoingDarkConfirmOpen] = useState(false);
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

  const getLookoutStateLabel = (state: LookoutState): string | null => {
    switch (state) {
      case LookoutState.SWEEPING:        return "Sweeping";
      case LookoutState.AWAITING_RETRY:  return "Retrying";
      case LookoutState.FALLEN:          return "Fallen";
      default: return null; // Idle/Active/Stopped/Unspecified — don't show badge
    }
  };

  const isPaused = session.status === SessionStatus.PAUSED;
  const isExternal = session.instanceType === InstanceType.EXTERNAL;
  const sourceTerminal = session.externalMetadata?.sourceTerminal || "External";
  const muxEnabled = session.externalMetadata?.muxEnabled || false;

  const handleCardClick = (e: React.MouseEvent) => {
    if (selectMode && onToggleSelect) {
      e.stopPropagation();
      onToggleSelect();
    } else if (onClick) {
      onClick();
    }
  };

  const handleCardKeyDown = (e: React.KeyboardEvent) => {
    // Support keyboard navigation with Enter or Space
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (selectMode && onToggleSelect) {
        onToggleSelect();
      } else if (onClick) {
        onClick();
      }
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
    setIsTagEditorOpen(true);
  };

  const handleSaveTags = (newTags: string[]) => {
    if (onUpdateTags) {
      onUpdateTags(session.id, newTags);
    }
    setIsTagEditorOpen(false);
  };

  const handleCancelTagEdit = () => {
    setIsTagEditorOpen(false);
  };

  const handleRenameClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setNewTitle(session.title);
    setRenameError("");
    setIsRenameOpen(true);
  };

  const handleRenameSubmit = async (e: React.MouseEvent) => {
    e.stopPropagation();

    // Validation
    if (!newTitle.trim()) {
      setRenameError("Title cannot be empty");
      return;
    }

    if (newTitle === session.title) {
      setIsRenameOpen(false);
      return;
    }

    setIsRenaming(true);
    setRenameError("");

    try {
      const success = await onRename?.(session.id, newTitle.trim());
      if (success) {
        setIsRenameOpen(false);
      } else {
        setRenameError("Failed to rename session");
      }
    } catch (error) {
      setRenameError(error instanceof Error ? error.message : "Failed to rename session");
    } finally {
      setIsRenaming(false);
    }
  };

  const handleRenameCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRenameOpen(false);
    setNewTitle(session.title);
    setRenameError("");
  };

  const handleRestartClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestartConfirmOpen(true);
  };

  const handleRestartConfirm = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestarting(true);

    try {
      await onRestart?.(session.id);
      setIsRestartConfirmOpen(false);
    } catch (error) {
      console.error("Failed to restart session:", error);
    } finally {
      setIsRestarting(false);
    }
  };

  const handleRestartCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRestartConfirmOpen(false);
  };

  // Shared helper — avoids duplicating the same try/catch/finally in enable and disable paths.
  const executeGoingDarkToggle = async (enabled: boolean) => {
    setIsTogglingDark(true);
    setToggleDarkError("");
    try {
      await onToggleGoingDark?.(session.id, enabled);
    } catch (error) {
      setToggleDarkError(error instanceof Error ? error.message : "Failed to toggle Going Dark mode");
    } finally {
      setIsTogglingDark(false);
    }
  };

  const handleGoingDarkToggleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!session.goingDark) {
      // Enabling: show confirmation dialog first
      setIsGoingDarkConfirmOpen(true);
    } else {
      // Disabling: proceed immediately, no confirmation needed
      void executeGoingDarkToggle(false);
    }
  };

  const handleGoingDarkConfirm = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsGoingDarkConfirmOpen(false);
    await executeGoingDarkToggle(true);
  };

  const handleGoingDarkCancel = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsGoingDarkConfirmOpen(false);
  };

  // Close the Going Dark confirmation dialog when the user presses Escape.
  useEffect(() => {
    if (!isGoingDarkConfirmOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setIsGoingDarkConfirmOpen(false);
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isGoingDarkConfirmOpen]);

  return (
    <>
      {isTagEditorOpen && (
        <TagEditor
          tags={session.tags || []}
          onSave={handleSaveTags}
          onCancel={handleCancelTagEdit}
          sessionTitle={session.title}
        />
      )}
      {isRenameOpen && (
        <div className={styles.renameDialog} onClick={(e) => e.stopPropagation()}>
          <div className={styles.dialogContent}>
            <h3>Rename Session</h3>
            <input
              type="text"
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleRenameSubmit(e as any);
                if (e.key === "Escape") handleRenameCancel(e as any);
              }}
              placeholder="Enter new title"
              autoFocus
              className={styles.renameInput}
            />
            {renameError && <span className={styles.errorMessage}>{renameError}</span>}
            <div className={styles.dialogActions}>
              <button
                onClick={handleRenameSubmit}
                disabled={isRenaming || !newTitle.trim()}
                className={styles.submitButton}
              >
                {isRenaming ? "Renaming..." : "Rename"}
              </button>
              <button
                onClick={handleRenameCancel}
                disabled={isRenaming}
                className={styles.cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
      {isRestartConfirmOpen && (
        <div className={styles.confirmDialog} onClick={(e) => e.stopPropagation()}>
          <div className={styles.dialogContent}>
            <h3>Restart Session</h3>
            <p>Are you sure you want to restart &quot;{session.title}&quot;?</p>
            <p className={styles.warningText}>This will terminate the current process and start a new one.</p>
            <div className={styles.dialogActions}>
              <button
                onClick={handleRestartConfirm}
                disabled={isRestarting}
                className={styles.dangerButton}
              >
                {isRestarting ? "Restarting..." : "Restart"}
              </button>
              <button
                onClick={handleRestartCancel}
                disabled={isRestarting}
                className={styles.cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
      {isGoingDarkConfirmOpen && (
        <div
          className={styles.confirmDialog}
          onClick={(e) => e.stopPropagation()}
          role="dialog"
          aria-modal="true"
          aria-labelledby={`going-dark-dialog-title-${session.id}`}
        >
          <div className={styles.dialogContent}>
            <h3 id={`going-dark-dialog-title-${session.id}`}>Enable Going Dark Mode</h3>
            <p>Enable autonomous correction mode for &quot;{session.title}&quot;?</p>
            <p className={styles.warningText}>
              In Going Dark mode, the Fixer will automatically inject correction prompts
              without asking for your approval. The session will fix failing tests on its own.
            </p>
            <div className={styles.dialogActions}>
              <button
                onClick={handleGoingDarkConfirm}
                disabled={isTogglingDark}
                className={styles.submitButton}
              >
                {isTogglingDark ? "Enabling..." : "🌙 Enable Going Dark"}
              </button>
              <button
                onClick={handleGoingDarkCancel}
                disabled={isTogglingDark}
                className={styles.cancelButton}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    <div
      className={`${styles.card} ${selectMode ? styles.selectMode : ""} ${isSelected ? styles.selected : ""} ${isExternal ? styles.external : ""} ${isDeleting ? styles.deleting : ""}`}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      role="button"
      tabIndex={0}
      aria-label={`Session ${session.title}, status: ${getStatusText(session.status)}, program: ${session.program}`}
      aria-pressed={selectMode ? isSelected : undefined}
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
            {isExternal && (
              <span
                className={styles.externalBadge}
                title={`External session from ${sourceTerminal}${muxEnabled ? " (mux-enabled)" : ""}`}
                aria-label={`External session from ${sourceTerminal}`}
              >
                🔗 {sourceTerminal}
                {muxEnabled && <span className={styles.muxIndicator}>✓</span>}
              </span>
            )}
            <GitHubBadge
              prNumber={session.githubPrNumber}
              prUrl={session.githubPrUrl}
              owner={session.githubOwner}
              repo={session.githubRepo}
              sourceRef={session.githubSourceRef}
              compact={true}
            />
            {reviewItem && (
              <ReviewQueueBadge
                priority={reviewItem.priority}
                reason={reviewItem.reason}
                compact={true}
              />
            )}
            {session.goingDark && (
              <span
                className={styles.goingDarkBadge}
                title="Going Dark: autonomous correction mode enabled"
                aria-label="Going Dark mode is enabled for this session"
              >
                🌙 Dark
              </span>
            )}
            {(() => {
              const lookoutLabel = getLookoutStateLabel(session.lookoutState);
              return lookoutLabel ? (
                <span
                  className={`${styles.lookoutBadge} ${session.lookoutState === LookoutState.FALLEN ? styles.lookoutBadgeFallen : ""}`}
                  title={`Lookout: ${lookoutLabel}`}
                  aria-label={`Lookout state: ${lookoutLabel}`}
                >
                  🔍 {lookoutLabel}
                </span>
              ) : null;
            })()}
            <span
              className={`${styles.status} ${getStatusColor(session.status)}`}
              role="status"
              aria-label={`Session status: ${getStatusText(session.status)}`}
            >
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
          {session.githubOwner && session.githubRepo && (
            <div className={styles.infoRow}>
              <span className={styles.label}>Repository:</span>
              <span className={styles.value}>
                <a
                  href={`https://github.com/${session.githubOwner}/${session.githubRepo}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className={styles.githubLink}
                >
                  {session.githubOwner}/{session.githubRepo}
                </a>
              </span>
            </div>
          )}
          {session.githubPrNumber > 0 && session.githubPrUrl && (
            <div className={styles.infoRow}>
              <span className={styles.label}>Pull Request:</span>
              <span className={styles.value}>
                <a
                  href={session.githubPrUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className={styles.githubLink}
                >
                  #{session.githubPrNumber}
                </a>
              </span>
            </div>
          )}
          {session.clonedRepoPath && (
            <div className={styles.infoRow}>
              <span className={styles.label}>Cloned To:</span>
              <span className={styles.value} title={session.clonedRepoPath}>
                {session.clonedRepoPath}
              </span>
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
            Created: <time dateTime={session.createdAt ? new Date(Number(session.createdAt.seconds) * 1000).toISOString() : ""}>{formatDate(session.createdAt)}</time>
          </span>
          <span className={styles.timestamp}>
            Updated: <time dateTime={session.updatedAt ? new Date(Number(session.updatedAt.seconds) * 1000).toISOString() : ""}>{formatDate(session.updatedAt)}</time>
          </span>
          {(() => {
            // Use the most recent of lastMeaningfulOutput and lastTerminalUpdate.
            // lastMeaningfulOutput is gated by a content-signature check, so it can lag
            // behind lastTerminalUpdate when content repeats (e.g. idle prompt).
            const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
            const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
            const lastActivity = moSecs === BigInt(0) && tuSecs === BigInt(0)
              ? undefined
              : moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
            return lastActivity ? (
              <span className={styles.timestamp} title="Last terminal activity">
                Last Activity: <time dateTime={new Date(Number(lastActivity.seconds) * 1000).toISOString()}>{formatTimeAgo(lastActivity)}</time>
              </span>
            ) : null;
          })()}
        </div>

          <button
            className={styles.actionsToggle}
            onClick={(e) => { e.stopPropagation(); setShowActions(!showActions); }}
            aria-expanded={showActions}
            aria-label="Toggle session actions"
          >
            Actions {showActions ? "▲" : "▼"}
          </button>
        <div className={`${styles.actions} ${showActions ? styles.actionsOpen : ""}`}>
          {isPaused ? (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResume?.();
              }}
              aria-label={`Resume session ${session.title}`}
              title="Resume this session"
            >
              ▶️ Resume
            </button>
          ) : (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onPause?.();
              }}
              aria-label={`Pause session ${session.title}`}
              title="Pause this session"
            >
              ⏸️ Pause
            </button>
          )}
          <button
            className={styles.actionButton}
            onClick={handleRenameClick}
            title="Rename this session"
            aria-label={`Rename session ${session.title}`}
          >
            ✏️ Rename
          </button>
          <button
            className={`${styles.actionButton} ${styles.restartButton}`}
            onClick={handleRestartClick}
            title="Restart this session"
            aria-label={`Restart session ${session.title}`}
          >
            🔄 Restart
          </button>
          <button
            className={styles.actionButton}
            onClick={(e) => {
              e.stopPropagation();
              onDuplicate?.();
            }}
            title="Duplicate this session with editable configuration"
            aria-label={`Duplicate session ${session.title}`}
          >
            📋 Duplicate
          </button>
          <button
            className={`${styles.actionButton} ${styles.deleteButton}`}
            onClick={async (e) => {
              e.stopPropagation();
              setIsDeleting(true);
              try {
                await onDelete?.();
              } catch {
                setIsDeleting(false);
              }
            }}
            disabled={isDeleting}
            aria-label={`Delete session ${session.title}`}
            title="Delete this session"
          >
            {isDeleting ? "Deleting..." : "🗑️ Delete"}
          </button>
          {onToggleGoingDark && (
            <>
              <button
                className={`${styles.actionButton} ${session.goingDark ? styles.goingDarkActive : styles.goingDarkButton}`}
                onClick={handleGoingDarkToggleClick}
                disabled={isTogglingDark}
                title={session.goingDark ? "Disable Going Dark (autonomous) mode" : "Enable Going Dark (autonomous) mode — requires confirmation"}
                aria-label={`${session.goingDark ? "Disable" : "Enable"} Going Dark mode for ${session.title}`}
                aria-pressed={session.goingDark}
              >
                {isTogglingDark
                    ? (session.goingDark ? "Disabling..." : "Enabling...")
                    : session.goingDark ? "🌙 Going Dark: ON" : "☀️ Going Dark: OFF"}
              </button>
              {toggleDarkError && <span className={styles.toggleDarkError}>{toggleDarkError}</span>}
            </>
          )}
        </div>
      </div>
    </div>
    </>
  );
}
