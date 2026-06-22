import { Button } from '../ui/Button';
import { Card } from '../ui/Card';
import { Badge } from '../ui/Badge';
import { PriceDisplay } from '../ui/PriceDisplay';
import type { ProductWithActivity } from '../../api/types';

interface ProductCardProps {
  product: ProductWithActivity;
  isSelected: boolean;
  onStartGroup: () => void;
  onDirectBuy: () => void;
  isLockPending: boolean;
}

export function ProductCard({
  product,
  isSelected,
  onStartGroup,
  onDirectBuy,
  isLockPending,
}: ProductCardProps) {
  const act = product.activity;
  const hasActivity = act != null;
  const deduction = act?.deduction_price;

  return (
    <Card
      padding="lg"
      className={isSelected ? 'ring-2 ring-[var(--color-accent)]' : ''}
    >
      {/* Top row: name + original price */}
      <div className="flex items-start justify-between mb-3">
        <h3 className="text-lg font-medium text-[var(--color-text-primary)]">
          {product.goods_name}
        </h3>
        <span className="text-sm text-[var(--color-text-muted)] tabular-nums">
          ¥{product.original_price}
        </span>
      </div>

      {/* Activity info area */}
      {hasActivity ? (
        <div className="mb-4">
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="warning">直降优惠</Badge>
            {deduction && deduction !== '0.00' && (
              <span className="text-xs text-[var(--color-accent)]">
                立省¥{deduction}
              </span>
            )}
            <span className="text-xs text-[var(--color-text-muted)]">
              {act.target_count}人成团
            </span>
          </div>
          <PriceDisplay
            originalPrice={product.original_price}
            payPrice={act.pay_price}
            deductionPrice={act.deduction_price}
          />
        </div>
      ) : (
        <div className="mb-4 py-3">
          <p className="text-sm text-[var(--color-text-muted)]">
            当前商品无活动
          </p>
        </div>
      )}

      {/* Action buttons */}
      <div className="flex gap-2">
        {hasActivity && (
          <Button
            size="sm"
            className="flex-1"
            onClick={onStartGroup}
            loading={isLockPending}
            disabled={isLockPending}
          >
            开团购买
          </Button>
        )}
        <Button
          variant={hasActivity ? 'secondary' : 'primary'}
          size="sm"
          className="flex-1"
          onClick={onDirectBuy}
          loading={isLockPending}
          disabled={isLockPending}
        >
          直接购买
        </Button>
      </div>
    </Card>
  );
}
