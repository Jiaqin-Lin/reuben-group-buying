import { Button } from './Button';

interface ErrorStateProps {
  message: string;
  onRetry?: () => void;
  className?: string;
}

export function ErrorState({ message, onRetry, className }: ErrorStateProps) {
  return (
    <div
      className={`flex flex-col items-center justify-center py-16 px-4 text-center ${className || ''}`}
    >
      <div className="w-12 h-12 rounded-full bg-[var(--color-pale-red)] flex items-center justify-center mb-4">
        <svg
          width="20"
          height="20"
          viewBox="0 0 20 20"
          fill="none"
          stroke="var(--color-pale-red-text)"
          strokeWidth="1.5"
          strokeLinecap="round"
        >
          <circle cx="10" cy="10" r="8" />
          <path d="M10 6v5M10 13.5v.5" />
        </svg>
      </div>
      <h3 className="text-base font-medium text-[var(--color-text-primary)] mb-2">
        出错了
      </h3>
      <p className="text-sm text-[var(--color-text-muted)] max-w-sm mb-4">
        {message}
      </p>
      {onRetry && (
        <Button variant="secondary" size="sm" onClick={onRetry}>
          重试
        </Button>
      )}
    </div>
  );
}
