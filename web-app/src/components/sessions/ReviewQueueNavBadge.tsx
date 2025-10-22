"use client";

import { useReviewQueue } from "@/lib/hooks/useReviewQueue";
import { getApiBaseUrl } from "@/lib/config";
import styles from "./ReviewQueueNavBadge.module.css";

interface ReviewQueueNavBadgeProps {
  inline?: boolean;
}

/**
 * Navigation badge that displays the count of items in the review queue.
 * Used in the header navigation to show queue status at a glance.
 */
export function ReviewQueueNavBadge({ inline = false }: ReviewQueueNavBadgeProps) {
  const { items, loading } = useReviewQueue({
    baseUrl: getApiBaseUrl(),
    autoRefresh: true,
    refreshInterval: 5000,
  });

  const count = items.length;

  // Always show badge (even when count is 0) for test visibility
  // Badge will be styled to show 0 state
  const className = inline
    ? `${styles.badge} ${styles.inline} ${count === 0 ? styles.empty : ""}`
    : `${styles.badge} ${count === 0 ? styles.empty : ""}`;

  return (
    <span
      className={className}
      data-testid="review-queue-badge"
      aria-label={`${count} item${count !== 1 ? "s" : ""} in review queue`}
      title={`${count} session${count !== 1 ? "s" : ""} ${count > 0 ? "need attention" : "- queue empty"}`}
    >
      {count}
    </span>
  );
}
