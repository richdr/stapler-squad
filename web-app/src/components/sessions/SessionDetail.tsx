"use client";

import { useState } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import { TerminalOutput } from "./TerminalOutput";
import { DiffViewer } from "./DiffViewer";
import styles from "./SessionDetail.module.css";

export type SessionDetailTab = "terminal" | "diff" | "logs" | "info";

interface SessionDetailProps {
  session: Session;
  onClose: () => void;
}

export function SessionDetail({ session, onClose }: SessionDetailProps) {
  const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");

  const tabs: { id: SessionDetailTab; label: string; icon: string }[] = [
    { id: "terminal", label: "Terminal", icon: "⌨️" },
    { id: "diff", label: "Diff", icon: "📝" },
    { id: "logs", label: "Logs", icon: "📋" },
    { id: "info", label: "Info", icon: "ℹ️" },
  ];

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2 className={styles.title}>{session.title}</h2>
        <button
          className={styles.closeButton}
          onClick={onClose}
          aria-label="Close"
        >
          ✕
        </button>
      </div>

      <div className={styles.tabs}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={`${styles.tab} ${activeTab === tab.id ? styles.active : ""}`}
            onClick={() => setActiveTab(tab.id)}
          >
            <span className={styles.tabIcon}>{tab.icon}</span>
            <span className={styles.tabLabel}>{tab.label}</span>
          </button>
        ))}
      </div>

      <div className={styles.content}>
        {activeTab === "terminal" && (
          <div className={styles.tabContent}>
            <TerminalOutput sessionId={session.id} baseUrl="http://localhost:8543" />
          </div>
        )}
        {activeTab === "diff" && (
          <div className={styles.tabContent}>
            <DiffViewer sessionId={session.id} baseUrl="http://localhost:8543" />
          </div>
        )}
        {activeTab === "logs" && (
          <div className={styles.tabContent}>
            <p className={styles.placeholder}>
              Session logs coming soon...
            </p>
          </div>
        )}
        {activeTab === "info" && (
          <div className={styles.tabContent}>
            <div className={styles.infoGrid}>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Session ID:</span>
                <span className={styles.infoValue}>{session.id}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Status:</span>
                <span className={styles.infoValue}>{session.status}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Branch:</span>
                <span className={styles.infoValue}>{session.branch}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Category:</span>
                <span className={styles.infoValue}>{session.category}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Created:</span>
                <span className={styles.infoValue}>
                  {session.createdAt ? new Date(Number(session.createdAt.seconds) * 1000).toLocaleString() : "N/A"}
                </span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>Updated:</span>
                <span className={styles.infoValue}>
                  {session.updatedAt ? new Date(Number(session.updatedAt.seconds) * 1000).toLocaleString() : "N/A"}
                </span>
              </div>
              {session.path && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Workspace Path:</span>
                  <span className={styles.infoValue}>{session.path}</span>
                </div>
              )}
              {session.workingDir && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Working Directory:</span>
                  <span className={styles.infoValue}>{session.workingDir}</span>
                </div>
              )}
              {session.program && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Program:</span>
                  <span className={styles.infoValue}>{session.program}</span>
                </div>
              )}
              {session.prompt && (
                <div className={styles.infoItem}>
                  <span className={styles.infoLabel}>Initial Prompt:</span>
                  <span className={styles.infoValue}>{session.prompt}</span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
