"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { routes } from "@/lib/routes";
import styles from "./Navigation.module.css";

export function Navigation() {
  const pathname = usePathname();

  const navItems = [
    { href: routes.home, label: "Sessions" },
    { href: routes.dashboard, label: "Dashboard" },
  ];

  return (
    <nav className={styles.nav} role="navigation" aria-label="Main navigation">
      <div className={styles.container}>
        <div className={styles.brand}>
          <Link href={routes.home} aria-label="Claude Squad home">
            <h1 className={styles.title}>Claude Squad</h1>
          </Link>
        </div>

        <ul className={styles.menu} role="menubar">
          {navItems.map((item) => (
            <li key={item.href} role="none">
              <Link
                href={item.href}
                role="menuitem"
                aria-current={pathname === item.href ? "page" : undefined}
                className={`${styles.link} ${
                  pathname === item.href ? styles.active : ""
                }`}
              >
                {item.label}
              </Link>
            </li>
          ))}
        </ul>

        <div className={styles.actions}>
          <Link
            href={routes.sessionCreate}
            className={styles.createButton}
            aria-label="Create new session"
          >
            New Session
          </Link>
        </div>
      </div>
    </nav>
  );
}
