"use client";

import { useState, useEffect } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { usePathname } from "next/navigation";
import { ReviewQueueNavBadge } from "@/components/sessions/ReviewQueueNavBadge";
import { ApprovalNavBadge } from "@/components/sessions/ApprovalNavBadge";
import { DebugMenu } from "@/components/ui/DebugMenu";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import styles from "./Header.module.css";

export function Header() {
  const pathname = usePathname();
  const [isDebugMenuOpen, setIsDebugMenuOpen] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const { togglePanel, getUnreadCount } = useNotifications();
  const { open: openOmnibar } = useOmnibar();
  const unreadCount = getUnreadCount();

  // Close mobile menu on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isMobileMenuOpen) {
        setIsMobileMenuOpen(false);
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isMobileMenuOpen]);

  // Close mobile menu on route change
  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  return (
    <>
      <header className={styles.header}>
      <div className={styles.container}>
        <div className={styles.branding}>
          <h1 className={styles.title}>Claude Squad</h1>
          <span className={styles.subtitle}>Session Manager</span>
        </div>

        <button
          className={styles.hamburger}
          aria-label={isMobileMenuOpen ? "Close navigation menu" : "Open navigation menu"}
          aria-expanded={isMobileMenuOpen}
          aria-controls="mobile-nav"
          onClick={() => setIsMobileMenuOpen((prev) => !prev)}
        >
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen1 : ""}`} />
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen2 : ""}`} />
          <span className={`${styles.hamburgerLine} ${isMobileMenuOpen ? styles.hamburgerLineOpen3 : ""}`} />
        </button>

        <nav
          id="mobile-nav"
          aria-label="Main navigation"
          className={`${styles.nav} ${isMobileMenuOpen ? styles.navOpen : ""}`}
        >
          <AppLink
            href="/"
            className={`${styles.navLink} ${pathname === "/" ? styles.active : ""}`}
          >
            Sessions
          </AppLink>
          <AppLink
            href="/review-queue"
            className={`${styles.navLink} ${pathname === "/review-queue" ? styles.active : ""}`}
          >
            <span className={styles.navLinkText}>Review Queue</span>
            <ReviewQueueNavBadge inline={true} />
          </AppLink>
          <AppLink
            href="/logs"
            className={`${styles.navLink} ${pathname === "/logs" ? styles.active : ""}`}
          >
            Logs
          </AppLink>
          <AppLink
            href="/history"
            className={`${styles.navLink} ${pathname === "/history" ? styles.active : ""}`}
          >
            History
          </AppLink>
          <AppLink
            href="/config"
            className={`${styles.navLink} ${pathname === "/config" ? styles.active : ""}`}
          >
            Config
          </AppLink>
        </nav>

        <div className={styles.actions}>
          <button
            className={styles.newSessionButton}
            onClick={openOmnibar}
            aria-label="Create new session (⌘K)"
            title="Create new session (⌘K)"
          >
            <span className={styles.newSessionIcon} aria-hidden="true">+</span>
            <span className={styles.newSessionLabel}>New Session</span>
          </button>
          <ApprovalNavBadge />
          <button
            className={styles.notificationButton}
            onClick={togglePanel}
            aria-label="Open notifications"
            title="Notifications"
          >
            <span aria-hidden="true">🔔</span>
            {unreadCount > 0 && (
              <span className={styles.notificationBadge} aria-label={`${unreadCount} unread`}>{unreadCount}</span>
            )}
          </button>
          <button
            className={styles.debugButton}
            onClick={() => setIsDebugMenuOpen(true)}
            aria-label="Open debug menu"
            title="Debug menu"
          >
            <span aria-hidden="true">🛠️</span>
          </button>
          <button
            className={styles.helpButton}
            onClick={() => {
              window.dispatchEvent(new KeyboardEvent("keydown", { key: "?" }));
            }}
            aria-label="Show keyboard shortcuts"
            title="Keyboard shortcuts (?)"
          >
            ?
          </button>
        </div>
      </div>
    </header>

      <DebugMenu
        isOpen={isDebugMenuOpen}
        onClose={() => setIsDebugMenuOpen(false)}
      />
    </>
  );
}
