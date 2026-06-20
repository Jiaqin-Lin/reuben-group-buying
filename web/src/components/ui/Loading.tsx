import { cn } from '../../utils/cn';

interface LoadingProps {
  className?: string;
  /** Number of skeleton lines to show */
  lines?: number;
}

export function Loading({ className, lines = 3 }: LoadingProps) {
  return (
    <div className={cn('flex flex-col gap-3 p-6', className)}>
      {Array.from({ length: lines }).map((_, i) => (
        <div
          key={i}
          className="h-4 bg-[var(--color-canvas)] rounded animate-pulse"
          style={{ width: `${100 - i * 15}%` }}
        />
      ))}
    </div>
  );
}

/** Full-page centered spinner */
export function PageLoading() {
  return (
    <div className="flex items-center justify-center min-h-[60vh]">
      <div className="flex flex-col items-center gap-3">
        <svg
          className="animate-spin h-8 w-8 text-[var(--color-text-muted)]"
          viewBox="0 0 24 24"
          fill="none"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
          />
        </svg>
        <span className="text-sm text-[var(--color-text-muted)]">
          加载中...
        </span>
      </div>
    </div>
  );
}
