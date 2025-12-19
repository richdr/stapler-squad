"use client";

import { useEffect, useRef, useCallback } from "react";
import { ReviewItem } from "@/gen/session/v1/types_pb";
import {
  playNotificationSound,
  showBrowserNotification,
  NotificationSound,
} from "@/lib/utils/notifications";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import {
  shouldNotify,
  markNotified,
  markAcknowledged,
  markNotifiedBatch,
  cleanupExpired,
} from "@/lib/utils/notificationStorage";

interface UseReviewQueueNotificationsOptions {
  /**
   * Enable/disable notifications
   * @default true
   */
  enabled?: boolean;

  /**
   * Sound type to play
   * @default NotificationSound.DING
   */
  soundType?: NotificationSound;

  /**
   * Show browser notification in addition to sound
   * @default true
   */
  showBrowserNotification?: boolean;

  /**
   * Show in-app toast notification
   * @default true
   */
  showToastNotification?: boolean;

  /**
   * Custom notification title
   */
  notificationTitle?: string;

  /**
   * Callback to navigate to a session when clicked
   */
  onNavigateToSession?: (sessionId: string) => void;

  /**
   * Callback when new items are detected
   */
  onNewItems?: (items: ReviewItem[]) => void;

  /**
   * Callback when a session is acknowledged from the notification toast.
   * This should call the backend acknowledge API.
   */
  onAcknowledge?: (sessionId: string) => void;
}

/**
 * Hook that monitors review queue items and plays notification sounds
 * when new sessions are added that need user attention.
 *
 * @example
 * ```tsx
 * const { items } = useReviewQueue();
 * useReviewQueueNotifications(items, {
 *   enabled: true,
 *   soundType: NotificationSound.DING,
 *   showBrowserNotification: true,
 * });
 * ```
 */
export function useReviewQueueNotifications(
  items: ReviewItem[],
  options: UseReviewQueueNotificationsOptions = {}
) {
  const {
    enabled = true,
    soundType = NotificationSound.DING,
    showBrowserNotification: showBrowser = true,
    showToastNotification: showToast = true,
    notificationTitle = "Session Needs Attention",
    onNewItems,
    onNavigateToSession,
    onAcknowledge,
  } = options;

  const { showSessionNotification } = useNotifications();

  // Track previous items to detect new additions (in-memory for fast access)
  const previousItemsRef = useRef<Set<string>>(new Set());
  const isInitialLoadRef = useRef(true);

  // Acknowledge handler that updates localStorage and calls backend
  const handleAcknowledge = useCallback(
    (sessionId: string) => {
      // Mark as acknowledged in localStorage (prevents re-notification for grace period)
      markAcknowledged(sessionId);
      // Call the backend acknowledge callback
      onAcknowledge?.(sessionId);
    },
    [onAcknowledge]
  );

  useEffect(() => {
    if (!enabled) return;

    // Periodic cleanup of expired records
    cleanupExpired();

    // Build current item set
    const currentItemIds = new Set(items.map((item) => item.sessionId));

    // On initial load, mark all current items as notified to prevent duplicate alerts
    // This handles both first page load AND WebSocket reconnection scenarios
    if (isInitialLoadRef.current) {
      isInitialLoadRef.current = false;
      // Mark all current items as notified in localStorage
      markNotifiedBatch(Array.from(currentItemIds));
      previousItemsRef.current = currentItemIds;
      return;
    }

    // Find new items that:
    // 1. Weren't in previous in-memory set
    // 2. Should be notified (not in localStorage grace period)
    const newItemIds = Array.from(currentItemIds).filter((id) => {
      // Not in previous set
      if (previousItemsRef.current.has(id)) {
        return false;
      }
      // Not in localStorage grace period or TTL
      return shouldNotify(id);
    });

    if (newItemIds.length > 0) {
      const newItems = items.filter((item) =>
        newItemIds.includes(item.sessionId)
      );

      // Mark all new items as notified in localStorage
      markNotifiedBatch(newItemIds);

      // Play notification sound (once for all new items)
      playNotificationSound(soundType);

      // Show toast notification for each new item with acknowledge action
      if (showToast && newItems.length > 0) {
        newItems.forEach((item) => {
          showSessionNotification(
            item,
            () => {
              onNavigateToSession?.(item.sessionId);
            },
            () => {
              handleAcknowledge(item.sessionId);
            }
          );
        });
      }

      // Show browser notification if enabled
      if (showBrowser && newItems.length > 0) {
        const sessionName = newItems[0].sessionName || "Unnamed Session";
        const body =
          newItems.length === 1
            ? `${sessionName} is waiting for your input`
            : `${newItems.length} sessions need your attention`;

        showBrowserNotification(notificationTitle, {
          body,
          tag: "review-queue", // Prevents duplicate browser notifications
          requireInteraction: false,
          silent: true, // We already played our custom sound
        });
      }

      // Call optional callback
      if (onNewItems) {
        onNewItems(newItems);
      }
    }

    // Update previous items reference
    previousItemsRef.current = currentItemIds;
  }, [
    items,
    enabled,
    soundType,
    showBrowser,
    showToast,
    notificationTitle,
    onNewItems,
    onNavigateToSession,
    handleAcknowledge,
  ]);

  return {
    // Reset tracking (useful if you want to re-enable after disabling)
    reset: () => {
      previousItemsRef.current = new Set();
      isInitialLoadRef.current = true;
    },
    // Manually acknowledge a session (for external use)
    acknowledge: handleAcknowledge,
    // Mark a session as notified (for external use)
    markNotified,
  };
}
