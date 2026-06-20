import { cn } from '../../utils/cn';
import { formatPrice } from '../../utils/format';

interface PriceDisplayProps {
  originalPrice: string;
  payPrice: string;
  deductionPrice?: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function PriceDisplay({
  originalPrice,
  payPrice,
  deductionPrice,
  size = 'md',
  className,
}: PriceDisplayProps) {
  const sizeStyles = {
    sm: { pay: 'text-lg', original: 'text-xs' },
    md: { pay: 'text-2xl', original: 'text-sm' },
    lg: { pay: 'text-3xl', original: 'text-base' },
  };

  return (
    <div className={cn('flex items-baseline gap-2', className)}>
      <span
        className={cn(
          'font-mono font-semibold text-[var(--color-accent)]',
          sizeStyles[size].pay,
        )}
      >
        {formatPrice(payPrice)}
      </span>
      <span
        className={cn(
          'font-mono text-[var(--color-text-muted)] line-through',
          sizeStyles[size].original,
        )}
      >
        {formatPrice(originalPrice)}
      </span>
      {deductionPrice && parseFloat(deductionPrice) > 0 && (
        <span className="text-xs font-medium text-[var(--color-pale-green-text)] bg-[var(--color-pale-green)] px-1.5 py-0.5 rounded-sm">
          省{formatPrice(deductionPrice)}
        </span>
      )}
    </div>
  );
}
