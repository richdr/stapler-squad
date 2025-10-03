import styles from "./KeyboardHint.module.css";

interface KeyboardHintProps {
  keys: string | string[];
  description: string;
  className?: string;
}

export function KeyboardHint({ keys, description, className = "" }: KeyboardHintProps) {
  const keyArray = Array.isArray(keys) ? keys : [keys];

  return (
    <div className={`${styles.hint} ${className}`}>
      <div className={styles.keys}>
        {keyArray.map((key, index) => (
          <span key={index}>
            <kbd className={styles.key}>{key}</kbd>
            {index < keyArray.length - 1 && (
              <span className={styles.separator}>+</span>
            )}
          </span>
        ))}
      </div>
      <span className={styles.description}>{description}</span>
    </div>
  );
}

interface KeyboardHintsProps {
  hints: Array<{
    keys: string | string[];
    description: string;
  }>;
  title?: string;
  className?: string;
}

export function KeyboardHints({ hints, title, className = "" }: KeyboardHintsProps) {
  return (
    <div className={`${styles.hintsContainer} ${className}`}>
      {title && <h3 className={styles.title}>{title}</h3>}
      <div className={styles.hints}>
        {hints.map((hint, index) => (
          <KeyboardHint
            key={index}
            keys={hint.keys}
            description={hint.description}
          />
        ))}
      </div>
    </div>
  );
}
