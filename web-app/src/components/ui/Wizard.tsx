"use client";

import { ReactNode } from "react";
import styles from "./Wizard.module.css";

interface WizardProps {
  currentStep: number;
  steps: string[];
  children: ReactNode;
}

export function Wizard({ currentStep, steps, children }: WizardProps) {
  return (
    <div className={styles.wizard}>
      <div className={styles.steps}>
        {steps.map((step, index) => (
          <div
            key={index}
            className={`${styles.step} ${
              index < currentStep
                ? styles.completed
                : index === currentStep
                ? styles.active
                : styles.pending
            }`}
          >
            <div className={styles.stepNumber}>
              {index < currentStep ? "✓" : index + 1}
            </div>
            <div className={styles.stepLabel}>{step}</div>
            {index < steps.length - 1 && <div className={styles.stepConnector} />}
          </div>
        ))}
      </div>
      <div className={styles.content}>{children}</div>
    </div>
  );
}

interface WizardActionsProps {
  children: ReactNode;
}

export function WizardActions({ children }: WizardActionsProps) {
  return <div className={styles.actions}>{children}</div>;
}
