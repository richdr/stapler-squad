/**
 * TerminalDimensionCache - localStorage persistence for terminal dimensions.
 *
 * Caches terminal dimensions per session to enable instant reconnection
 * without waiting for size stability detection. Pure utility functions
 * with no React dependencies.
 */

export interface CachedDimensions {
  cols: number;
  rows: number;
}

/**
 * Retrieve cached terminal dimensions for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @returns The cached dimensions, or null if not found or on error
 */
export function getCachedDimensions(sessionId: string): CachedDimensions | null {
  if (typeof window === 'undefined') return null;
  try {
    const key = `terminal-dimensions-${sessionId}`;
    const cached = localStorage.getItem(key);
    if (cached) {
      const dims = JSON.parse(cached) as CachedDimensions;
      console.log(`[TerminalDimensionCache] Loaded cached dimensions for ${sessionId}: ${dims.cols}x${dims.rows}`);
      return dims;
    }
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to load cached dimensions:', err);
  }
  return null;
}

/**
 * Save terminal dimensions to localStorage for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 */
export function saveDimensions(sessionId: string, cols: number, rows: number): void {
  if (typeof window === 'undefined') return;
  try {
    const key = `terminal-dimensions-${sessionId}`;
    localStorage.setItem(key, JSON.stringify({ cols, rows }));
    console.log(`[TerminalDimensionCache] Saved dimensions for ${sessionId}: ${cols}x${rows}`);
  } catch (err) {
    console.warn('[TerminalDimensionCache] Failed to save dimensions:', err);
  }
}
