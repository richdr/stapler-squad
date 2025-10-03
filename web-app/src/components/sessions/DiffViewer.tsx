"use client";

import { useState, useEffect } from "react";
import styles from "./DiffViewer.module.css";

interface DiffViewerProps {
  sessionId: string;
  baseUrl: string;
}

interface DiffFile {
  filename: string;
  additions: number;
  deletions: number;
  changes: DiffHunk[];
}

interface DiffHunk {
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  lines: DiffLine[];
}

interface DiffLine {
  type: "add" | "delete" | "context";
  content: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}

export function DiffViewer({ sessionId, baseUrl }: DiffViewerProps) {
  const [diff, setDiff] = useState<DiffFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"split" | "unified">("unified");

  // Placeholder for fetching diff data
  useEffect(() => {
    setLoading(true);
    // Simulate loading
    setTimeout(() => {
      // Placeholder diff data
      setDiff([
        {
          filename: "example.ts",
          additions: 15,
          deletions: 8,
          changes: [
            {
              oldStart: 10,
              oldLines: 5,
              newStart: 10,
              newLines: 7,
              lines: [
                { type: "context", content: "function example() {", oldLineNumber: 10, newLineNumber: 10 },
                { type: "delete", content: "  const oldCode = true;", oldLineNumber: 11 },
                { type: "add", content: "  const newCode = true;", newLineNumber: 11 },
                { type: "add", content: "  const moreCode = false;", newLineNumber: 12 },
                { type: "context", content: "  return result;", oldLineNumber: 12, newLineNumber: 13 },
                { type: "context", content: "}", oldLineNumber: 13, newLineNumber: 14 },
              ],
            },
          ],
        },
      ]);
      setLoading(false);
    }, 500);
  }, [sessionId, baseUrl]);

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading diff...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>{error}</div>
      </div>
    );
  }

  if (diff.length === 0) {
    return (
      <div className={styles.container}>
        <div className={styles.empty}>
          <p>No changes to display</p>
          <p className={styles.emptyHint}>
            Diff will show here when there are uncommitted changes in the session.
          </p>
        </div>
      </div>
    );
  }

  const totalAdditions = diff.reduce((sum, file) => sum + file.additions, 0);
  const totalDeletions = diff.reduce((sum, file) => sum + file.deletions, 0);

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <div className={styles.stats}>
          <span className={styles.filesChanged}>{diff.length} files changed</span>
          <span className={styles.additions}>+{totalAdditions}</span>
          <span className={styles.deletions}>-{totalDeletions}</span>
        </div>
        <div className={styles.viewModeToggle}>
          <button
            className={`${styles.viewModeButton} ${viewMode === "unified" ? styles.active : ""}`}
            onClick={() => setViewMode("unified")}
          >
            Unified
          </button>
          <button
            className={`${styles.viewModeButton} ${viewMode === "split" ? styles.active : ""}`}
            onClick={() => setViewMode("split")}
          >
            Split
          </button>
        </div>
      </div>

      <div className={styles.diffContent}>
        {diff.map((file, fileIndex) => (
          <div key={fileIndex} className={styles.file}>
            <div className={styles.fileHeader}>
              <span className={styles.filename}>{file.filename}</span>
              <span className={styles.fileStats}>
                <span className={styles.additions}>+{file.additions}</span>
                <span className={styles.deletions}>-{file.deletions}</span>
              </span>
            </div>

            {file.changes.map((hunk, hunkIndex) => (
              <div key={hunkIndex} className={styles.hunk}>
                <div className={styles.hunkHeader}>
                  @@ -{hunk.oldStart},{hunk.oldLines} +{hunk.newStart},{hunk.newLines} @@
                </div>
                <div className={styles.lines}>
                  {hunk.lines.map((line, lineIndex) => (
                    <div
                      key={lineIndex}
                      className={`${styles.line} ${styles[line.type]}`}
                    >
                      {viewMode === "unified" && (
                        <>
                          <span className={styles.lineNumber}>
                            {line.oldLineNumber || " "}
                          </span>
                          <span className={styles.lineNumber}>
                            {line.newLineNumber || " "}
                          </span>
                        </>
                      )}
                      <span className={styles.lineContent}>{line.content}</span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        ))}
      </div>

      <div className={styles.placeholder}>
        <p>Full diff visualization coming soon...</p>
        <ul>
          <li>Real-time diff updates via API</li>
          <li>Syntax highlighting for different languages</li>
          <li>Inline diff comments</li>
          <li>File tree navigation</li>
          <li>Expand/collapse hunks</li>
        </ul>
      </div>
    </div>
  );
}
