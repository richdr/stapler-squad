import styles from "./Skeleton.module.css";

interface SkeletonProps {
  className?: string;
  width?: string | number;
  height?: string | number;
  variant?: "text" | "circular" | "rectangular";
  style?: React.CSSProperties;
}

export function Skeleton({
  className = "",
  width,
  height,
  variant = "rectangular",
  style: customStyle,
}: SkeletonProps) {
  const style: React.CSSProperties = { ...customStyle };

  if (width) {
    style.width = typeof width === "number" ? `${width}px` : width;
  }
  if (height) {
    style.height = typeof height === "number" ? `${height}px` : height;
  }

  return (
    <div
      className={`${styles.skeleton} ${styles[variant]} ${className}`}
      style={style}
    />
  );
}
