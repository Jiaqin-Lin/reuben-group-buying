import { cn } from '../../utils/cn';

type BadgeVariant =
  | 'default'
  | 'success'
  | 'error'
  | 'warning'
  | 'info'
  | 'neutral';

interface BadgeProps {
  variant?: BadgeVariant;
  children: React.ReactNode;
  className?: string;
}

const variantStyles: Record<BadgeVariant, string> = {
  default: 'bg-[var(--color-pale-gray)] text-[var(--color-text-secondary)]',
  success: 'bg-[var(--color-pale-green)] text-[var(--color-pale-green-text)]',
  error: 'bg-[var(--color-pale-red)] text-[var(--color-pale-red-text)]',
  warning: 'bg-[var(--color-pale-yellow)] text-[var(--color-pale-yellow-text)]',
  info: 'bg-[var(--color-pale-blue)] text-[var(--color-pale-blue-text)]',
  neutral: 'bg-[var(--color-pale-gray)] text-[var(--color-text-secondary)]',
};

export function Badge({
  variant = 'default',
  className,
  children,
}: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium tracking-wider uppercase',
        variantStyles[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}
