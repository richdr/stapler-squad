"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Wizard, WizardActions } from "@/components/ui/Wizard";
import { sessionSchema, SessionFormData, defaultValues } from "@/lib/validation/sessionSchema";
import styles from "./SessionWizard.module.css";

interface SessionWizardProps {
  onComplete: (data: SessionFormData) => Promise<void>;
  onCancel: () => void;
}

export function SessionWizard({ onComplete, onCancel }: SessionWizardProps) {
  const [step, setStep] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors },
    trigger,
  } = useForm<SessionFormData>({
    resolver: zodResolver(sessionSchema),
    defaultValues,
    mode: "onChange",
  });

  const steps = ["Basic Info", "Repository", "Configuration"];

  const stepFields: Array<Array<keyof SessionFormData>> = [
    ["title", "category"],
    ["path", "workingDir", "branch"],
    ["program", "prompt", "autoYes"],
  ];

  const validateStep = async () => {
    const fields = stepFields[step];
    const isValid = await trigger(fields);
    return isValid;
  };

  const handleNext = async () => {
    const isValid = await validateStep();
    if (isValid && step < steps.length - 1) {
      setStep(step + 1);
    }
  };

  const handleBack = () => {
    if (step > 0) {
      setStep(step - 1);
    }
  };

  const onSubmit = async (data: SessionFormData) => {
    setIsSubmitting(true);
    try {
      await onComplete(data);
    } catch (error) {
      console.error("Failed to create session:", error);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Wizard currentStep={step} steps={steps}>
      <form onSubmit={handleSubmit(onSubmit)}>
        {step === 0 && (
          <div className={styles.step}>
            <h2>Basic Information</h2>
            <p className={styles.description}>
              Give your session a name and optionally organize it with a category.
            </p>

            <div className={styles.field}>
              <label htmlFor="title">
                Session Title <span className={styles.required}>*</span>
              </label>
              <input
                id="title"
                type="text"
                {...register("title")}
                placeholder="My Feature Branch"
                className={errors.title ? styles.error : ""}
              />
              {errors.title && (
                <span className={styles.errorMessage}>{errors.title.message}</span>
              )}
            </div>

            <div className={styles.field}>
              <label htmlFor="category">Category</label>
              <input
                id="category"
                type="text"
                {...register("category")}
                placeholder="Features, Bugfixes, Experiments..."
              />
              <span className={styles.hint}>
                Optional: Organize sessions into categories
              </span>
            </div>
          </div>
        )}

        {step === 1 && (
          <div className={styles.step}>
            <h2>Repository Setup</h2>
            <p className={styles.description}>
              Configure the git repository and working directory for this session.
            </p>

            <div className={styles.field}>
              <label htmlFor="path">
                Repository Path <span className={styles.required}>*</span>
              </label>
              <input
                id="path"
                type="text"
                {...register("path")}
                placeholder="/path/to/repository"
                className={errors.path ? styles.error : ""}
              />
              {errors.path && (
                <span className={styles.errorMessage}>{errors.path.message}</span>
              )}
              <span className={styles.hint}>
                Absolute path to your git repository
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="workingDir">Working Directory</label>
              <input
                id="workingDir"
                type="text"
                {...register("workingDir")}
                placeholder="subdirectory (optional)"
              />
              <span className={styles.hint}>
                Subdirectory within repository to start in
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="branch">Git Branch</label>
              <input
                id="branch"
                type="text"
                {...register("branch")}
                placeholder="feature/my-feature"
              />
              <span className={styles.hint}>
                Branch name (will create new worktree if specified)
              </span>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className={styles.step}>
            <h2>Configuration</h2>
            <p className={styles.description}>
              Configure the AI assistant program and optional startup settings.
            </p>

            <div className={styles.field}>
              <label htmlFor="program">Program</label>
              <select id="program" {...register("program")}>
                <option value="claude">Claude Code</option>
                <option value="aider">Aider</option>
                <option value="aider --model ollama_chat/gemma3:1b">
                  Aider (Ollama)
                </option>
              </select>
              <span className={styles.hint}>
                AI assistant to run in this session
              </span>
            </div>

            <div className={styles.field}>
              <label htmlFor="prompt">Initial Prompt</label>
              <textarea
                id="prompt"
                {...register("prompt")}
                placeholder="Optional: Initial message to send to the AI"
                rows={3}
              />
              <span className={styles.hint}>
                Optional: Message sent when session starts
              </span>
            </div>

            <div className={styles.field}>
              <label className={styles.checkbox}>
                <input type="checkbox" {...register("autoYes")} />
                <span>Auto-approve prompts (experimental mode)</span>
              </label>
              <span className={styles.hint}>
                Automatically approve all AI suggestions without confirmation
              </span>
            </div>
          </div>
        )}

        <WizardActions>
          {step > 0 && (
            <button
              type="button"
              onClick={handleBack}
              className={styles.buttonSecondary}
              disabled={isSubmitting}
            >
              Back
            </button>
          )}
          <button
            type="button"
            onClick={onCancel}
            className={styles.buttonSecondary}
            disabled={isSubmitting}
          >
            Cancel
          </button>
          {step < steps.length - 1 ? (
            <button
              type="button"
              onClick={handleNext}
              className={styles.buttonPrimary}
            >
              Next
            </button>
          ) : (
            <button
              type="submit"
              className={styles.buttonPrimary}
              disabled={isSubmitting}
            >
              {isSubmitting ? "Creating..." : "Create Session"}
            </button>
          )}
        </WizardActions>
      </form>
    </Wizard>
  );
}
