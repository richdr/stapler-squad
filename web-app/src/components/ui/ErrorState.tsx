import { ErrorInfo } from "react";
import styles from "./ErrorState.module.css";

interface ErrorStateProps {
  error?: Error | null;
  title?: string;
  message?: string;
  onRetry?: () => void;
  showDetails?: boolean;
  errorInfo?: ErrorInfo | null;
  actionLabel?: string;
}

export function ErrorState({
  error,
  title = "Error",
  message = "An error occurred",
  onRetry,
  showDetails = false,
  errorInfo,
  actionLabel = "Try Again",
}: ErrorStateProps) {
  return (
    <div className={styles.container}>
      <div className={styles.content}>
        <div className={styles.icon}>
          <svg
            width="64"
            height="64"
            viewBox="0 0 24 24"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <circle
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="2"
            />
            <path
              d="M12 8V12"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            />
            <circle cx="12" cy="16" r="1" fill="currentColor" />
          </svg>
        </div>

        <h2 className={styles.title}>{title}</h2>
        <p className={styles.message}>{message}</p>

        {showDetails && error && (
          <details className={styles.details}>
            <summary className={styles.detailsSummary}>Error Details</summary>
            <div className={styles.detailsContent}>
              <div className={styles.errorBlock}>
                <strong>Error:</strong>
                <pre className={styles.errorText}>{error.message}</pre>
              </div>

              {error.stack && (
                <div className={styles.errorBlock}>
                  <strong>Stack Trace:</strong>
                  <pre className={styles.stackTrace}>{error.stack}</pre>
                </div>
              )}

              {errorInfo?.componentStack && (
                <div className={styles.errorBlock}>
                  <strong>Component Stack:</strong>
                  <pre className={styles.stackTrace}>
                    {errorInfo.componentStack}
                  </pre>
                </div>
              )}
            </div>
          </details>
        )}

        {onRetry && (
          <button className={styles.retryButton} onClick={onRetry}>
            {actionLabel}
          </button>
        )}
      </div>
    </div>
  );
}
