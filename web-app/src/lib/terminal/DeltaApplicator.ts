/**
 * Delta Applicator for Terminal State Synchronization
 *
 * Applies MOSH-style delta compression protocol to xterm.js terminal.
 * Reduces bandwidth by 70-90% by only sending screen changes instead of raw output.
 *
 * Protocol:
 * - Server sends TerminalDelta messages with only changed lines
 * - Client applies deltas to maintain synchronized terminal state
 * - Version tracking prevents desynchronization
 * - Full sync fallback for recovery
 */

import { Terminal } from 'xterm';
import { TerminalDelta, LineDelta, CursorPosition, TerminalDimensions } from '@/gen/session/v1/events_pb';

/**
 * DeltaApplicator applies terminal state deltas to xterm.js terminal.
 */
export class DeltaApplicator {
  private terminal: Terminal;
  private currentVersion: bigint = BigInt(0);

  constructor(terminal: Terminal) {
    this.terminal = terminal;
  }

  /**
   * Apply a terminal delta to the terminal.
   *
   * @param delta - The delta to apply
   * @returns true if applied successfully, false if desync detected
   */
  applyDelta(delta: TerminalDelta): boolean {
    // Check for version mismatch (desynchronization)
    if (delta.fromState !== this.currentVersion && !delta.fullSync) {
      console.warn(
        `[DeltaApplicator] State desync: expected ${this.currentVersion}, got ${delta.fromState}. Requesting full sync.`
      );
      return false; // Signal caller to request full sync
    }

    // Handle full sync (initial state or recovery)
    if (delta.fullSync) {
      console.log('[DeltaApplicator] Applying full sync');
      this.applyFullSync(delta);
      this.currentVersion = delta.toState;
      return true;
    }

    // Handle dimension changes
    if (delta.dimensions) {
      this.applyDimensionChange(delta.dimensions);
    }

    // Apply line changes
    for (const lineDelta of delta.lines) {
      this.applyLineDelta(lineDelta);
    }

    // Update cursor position
    if (delta.cursor) {
      this.applyCursorPosition(delta.cursor);
    }

    // Update version tracking
    this.currentVersion = delta.toState;
    return true;
  }

  /**
   * Apply a full sync (complete terminal state).
   */
  private applyFullSync(delta: TerminalDelta): void {
    // Resize if dimensions provided
    if (delta.dimensions) {
      const rows = Number(delta.dimensions.rows);
      const cols = Number(delta.dimensions.cols);
      if (this.terminal.rows !== rows || this.terminal.cols !== cols) {
        this.terminal.resize(cols, rows);
      }
    }

    // Clear terminal
    this.terminal.clear();

    // Write all lines
    for (const lineDelta of delta.lines) {
      const lineNum = Number(lineDelta.lineNumber);

      // Get line content
      let lineText = '';
      if (lineDelta.operation.case === 'replaceLine') {
        lineText = lineDelta.operation.value;
      }

      // Position cursor and write line
      this.terminal.write(`\x1b[${lineNum + 1};1H${lineText}`);
    }

    // Update cursor
    if (delta.cursor) {
      this.applyCursorPosition(delta.cursor);
    }
  }

  /**
   * Apply dimension changes (terminal resize).
   */
  private applyDimensionChange(dimensions: TerminalDimensions): void {
    const rows = Number(dimensions.rows);
    const cols = Number(dimensions.cols);

    if (this.terminal.rows !== rows || this.terminal.cols !== cols) {
      console.log(`[DeltaApplicator] Resizing terminal to ${cols}x${rows}`);
      this.terminal.resize(cols, rows);
    }
  }

  /**
   * Apply changes to a specific line.
   */
  private applyLineDelta(lineDelta: LineDelta): void {
    const lineNum = Number(lineDelta.lineNumber);

    // Validate line number
    if (lineNum < 0 || lineNum >= this.terminal.rows) {
      console.warn(`[DeltaApplicator] Invalid line number: ${lineNum} (rows: ${this.terminal.rows})`);
      return;
    }

    switch (lineDelta.operation.case) {
      case 'replaceLine': {
        // Replace entire line
        const text = lineDelta.operation.value;
        // Move cursor to line, clear it, and write new content
        this.terminal.write(`\x1b[${lineNum + 1};1H\x1b[2K${text}`);
        break;
      }

      case 'edit': {
        // Character-level edit within line
        const edit = lineDelta.operation.value;
        const startCol = Number(edit.startCol);
        const text = edit.text;
        // Move cursor to position and write text (overwrites existing)
        this.terminal.write(`\x1b[${lineNum + 1};${startCol + 1}H${text}`);
        break;
      }

      case 'deleteLine': {
        // Delete line (shift lines up)
        // Move to line and delete it
        this.terminal.write(`\x1b[${lineNum + 1};1H\x1b[M`);
        break;
      }

      case 'insert': {
        // Insert new line (shift lines down)
        const insert = lineDelta.operation.value;
        const text = insert.text;
        // Move to line, insert blank line, then write text
        this.terminal.write(`\x1b[${lineNum + 1};1H\x1b[L${text}`);
        break;
      }

      case 'clearLine': {
        // Clear line to empty
        this.terminal.write(`\x1b[${lineNum + 1};1H\x1b[2K`);
        break;
      }

      default:
        console.warn(`[DeltaApplicator] Unknown line operation: ${lineDelta.operation.case}`);
    }
  }

  /**
   * Apply cursor position update.
   */
  private applyCursorPosition(cursor: CursorPosition): void {
    const row = Number(cursor.row);
    const col = Number(cursor.col);

    // Move cursor to position
    this.terminal.write(`\x1b[${row + 1};${col + 1}H`);

    // Handle cursor visibility
    if (cursor.visible) {
      this.terminal.write('\x1b[?25h'); // Show cursor
    } else {
      this.terminal.write('\x1b[?25l'); // Hide cursor
    }
  }

  /**
   * Get current version for synchronization.
   */
  getCurrentVersion(): bigint {
    return this.currentVersion;
  }

  /**
   * Reset version tracking (for recovery).
   */
  resetVersion(): void {
    this.currentVersion = BigInt(0);
  }
}
