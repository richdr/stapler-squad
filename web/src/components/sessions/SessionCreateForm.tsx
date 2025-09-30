"use client";

import { useState } from "react";
import { CreateSessionRequest } from "@/gen/session/v1/session_pb";
import styles from "./SessionCreateForm.module.css";

interface SessionCreateFormProps {
  onSubmit: (request: Partial<CreateSessionRequest>) => Promise<void>;
  onCancel: () => void;
}

export function SessionCreateForm({ onSubmit, onCancel }: SessionCreateFormProps) {
  const [formData, setFormData] = useState({
    title: "",
    path: "",
    workingDir: "",
    branch: "",
    program: "claude",
    category: "",
    prompt: "",
    autoYes: false,
    existingWorktree: "",
  });

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    // Validation
    if (!formData.title.trim()) {
      setError("Session title is required");
      return;
    }

    if (!formData.path.trim()) {
      setError("Repository path is required");
      return;
    }

    setSubmitting(true);

    try {
      await onSubmit({
        title: formData.title.trim(),
        path: formData.path.trim(),
        workingDir: formData.workingDir.trim() || undefined,
        branch: formData.branch.trim() || undefined,
        program: formData.program.trim() || undefined,
        category: formData.category.trim() || undefined,
        prompt: formData.prompt.trim() || undefined,
        autoYes: formData.autoYes,
        existingWorktree: formData.existingWorktree.trim() || undefined,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create session");
      setSubmitting(false);
    }
  };

  const handleChange = (field: string, value: string | boolean) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2>Create New Session</h2>
          <button
            className={styles.closeButton}
            onClick={onCancel}
            disabled={submitting}
          >
            ✕
          </button>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          {error && (
            <div className={styles.error}>
              <p>{error}</p>
            </div>
          )}

          <div className={styles.formGroup}>
            <label htmlFor="title" className={styles.label}>
              Session Title <span className={styles.required}>*</span>
            </label>
            <input
              id="title"
              type="text"
              value={formData.title}
              onChange={(e) => handleChange("title", e.target.value)}
              placeholder="my-feature-session"
              className={styles.input}
              disabled={submitting}
              autoFocus
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="path" className={styles.label}>
              Repository Path <span className={styles.required}>*</span>
            </label>
            <input
              id="path"
              type="text"
              value={formData.path}
              onChange={(e) => handleChange("path", e.target.value)}
              placeholder="/path/to/repository"
              className={styles.input}
              disabled={submitting}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="branch" className={styles.label}>
              Git Branch
            </label>
            <input
              id="branch"
              type="text"
              value={formData.branch}
              onChange={(e) => handleChange("branch", e.target.value)}
              placeholder="feature/my-feature"
              className={styles.input}
              disabled={submitting}
            />
            <p className={styles.hint}>
              Leave empty to use current branch, or specify a new branch name
            </p>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="workingDir" className={styles.label}>
              Working Directory
            </label>
            <input
              id="workingDir"
              type="text"
              value={formData.workingDir}
              onChange={(e) => handleChange("workingDir", e.target.value)}
              placeholder="subdirectory (optional)"
              className={styles.input}
              disabled={submitting}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="program" className={styles.label}>
              Program
            </label>
            <select
              id="program"
              value={formData.program}
              onChange={(e) => handleChange("program", e.target.value)}
              className={styles.select}
              disabled={submitting}
            >
              <option value="claude">Claude Code</option>
              <option value="aider">Aider</option>
              <option value="cursor">Cursor</option>
              <option value="custom">Custom</option>
            </select>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="category" className={styles.label}>
              Category
            </label>
            <input
              id="category"
              type="text"
              value={formData.category}
              onChange={(e) => handleChange("category", e.target.value)}
              placeholder="feature, bugfix, refactor, etc."
              className={styles.input}
              disabled={submitting}
            />
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="prompt" className={styles.label}>
              Initial Prompt
            </label>
            <textarea
              id="prompt"
              value={formData.prompt}
              onChange={(e) => handleChange("prompt", e.target.value)}
              placeholder="Instructions for the AI agent..."
              className={styles.textarea}
              disabled={submitting}
              rows={4}
            />
          </div>

          <div className={styles.formGroup}>
            <label className={styles.checkbox}>
              <input
                type="checkbox"
                checked={formData.autoYes}
                onChange={(e) => handleChange("autoYes", e.target.checked)}
                disabled={submitting}
              />
              <span>Auto-confirm prompts (--yes flag)</span>
            </label>
          </div>

          <div className={styles.formGroup}>
            <label htmlFor="existingWorktree" className={styles.label}>
              Existing Worktree Path
            </label>
            <input
              id="existingWorktree"
              type="text"
              value={formData.existingWorktree}
              onChange={(e) => handleChange("existingWorktree", e.target.value)}
              placeholder="/path/to/existing/worktree (optional)"
              className={styles.input}
              disabled={submitting}
            />
            <p className={styles.hint}>
              Use an existing git worktree instead of creating a new one
            </p>
          </div>

          <div className={styles.actions}>
            <button
              type="button"
              onClick={onCancel}
              className={styles.cancelButton}
              disabled={submitting}
            >
              Cancel
            </button>
            <button
              type="submit"
              className={styles.submitButton}
              disabled={submitting}
            >
              {submitting ? "Creating..." : "Create Session"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
