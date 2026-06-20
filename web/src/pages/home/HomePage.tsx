import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import QRCode from 'qrcode';
import { useAuth } from '../../context/AuthContext';
import { useToast } from '../../context/ToastContext';
import { useLockOrder } from '../../hooks/useLockOrder';
import { teamApi, productApi } from '../../api/index';
import { tradeApi } from '../../api/trade';
import { orderApi } from '../../api/admin';
import { Button } from '../../components/ui/Button';
import { Modal } from '../../components/ui/Modal';
import { PriceDisplay } from '../../components/ui/PriceDisplay';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { ProductCard } from '../../components/home/ProductCard';
import { TeamList } from '../../components/home/TeamList';
import { DEFAULT_SOURCE, DEFAULT_CHANNEL, POLL_INTERVAL, PAYMENT_TTL } from '../../utils/constants';
import { getErrorMessage } from '../../utils/constants';
import { formatCountdown, generateOutTradeNo } from '../../utils/format';
import type { LockResult, ProductWithActivity } from '../../api/types';


export function HomePage() {
  const { userId, isLoggedIn } = useAuth();
  const navigate = useNavigate();
  const { addToast } = useToast();
  const queryClient = useQueryClient();
  const lockMutation = useLockOrder();

  const [selectedProduct, setSelectedProduct] = useState<ProductWithActivity | null>(null);
  const [lockResult, setLockResult] = useState<LockResult | null>(null);
  const [showPayment, setShowPayment] = useState(false);
  const [paymentConfirmClose, setPaymentConfirmClose] = useState(false);
  const [countdown, setCountdown] = useState('');
  const [joiningTeamId, setJoiningTeamId] = useState<string | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const paymentEndRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Product list
  const {
    data: productsData,
    isLoading: productsLoading,
    isError: productsError,
    error: productsErr,
    refetch: refetchProducts,
  } = useQuery({
    queryKey: ['products'],
    queryFn: () => productApi.list(),
    enabled: isLoggedIn && !!userId,
  });
  const products = productsData?.products || [];

  // Teams list for selected product
  const { data: teamsData, isLoading: teamsLoading, refetch: refetchTeams } = useQuery({
    queryKey: ['teams', selectedProduct?.activity?.activity_id],
    queryFn: () => teamApi.listByActivity(selectedProduct!.activity!.activity_id, 1, 20),
    enabled: !!selectedProduct?.activity?.activity_id,
  });

  // User's orders for "已加入" indicator
  const { data: myOrders } = useQuery({
    queryKey: ['my-orders', userId],
    queryFn: () => orderApi.listByUser(userId!),
    enabled: isLoggedIn && !!userId,
  });
  const myTeamIds = new Set(
    (myOrders || [])
      .filter((o) => o.activity_id === selectedProduct?.activity?.activity_id && o.status !== 2)
      .map((o) => o.team_id),
  );

  // Auto-select first product on load
  useEffect(() => {
    if (products.length > 0 && !selectedProduct) {
      setSelectedProduct(products[0]);
    }
  }, [products, selectedProduct]);

  // Generate QR code
  useEffect(() => {
    if (!lockResult?.pay_url) {
      setQrDataUrl(null);
      return;
    }
    QRCode.toDataURL(lockResult.pay_url, { width: 280, margin: 1 })
      .then(setQrDataUrl)
      .catch(() => setQrDataUrl(null));
  }, [lockResult?.pay_url]);

  // Countdown timer
  useEffect(() => {
    if (!showPayment) {
      setCountdown('');
      return;
    }
    paymentEndRef.current = Date.now() + PAYMENT_TTL * 1000;
    setCountdown(formatCountdown(PAYMENT_TTL));

    countdownTimerRef.current = setInterval(() => {
      const left = Math.max(0, Math.floor((paymentEndRef.current - Date.now()) / 1000));
      setCountdown(formatCountdown(left));
      if (left <= 0 && countdownTimerRef.current) {
        clearInterval(countdownTimerRef.current);
      }
    }, 1000);

    return () => {
      if (countdownTimerRef.current) {
        clearInterval(countdownTimerRef.current);
        countdownTimerRef.current = null;
      }
    };
  }, [showPayment]);

  // Poll payment status
  const lockResultRef = useRef(lockResult);
  lockResultRef.current = lockResult;

  const refreshAll = useCallback(() => {
    refetchProducts();
    refetchTeams();
    queryClient.invalidateQueries({ queryKey: ['my-orders', userId] });
  }, [refetchProducts, refetchTeams, queryClient, userId]);

  useEffect(() => {
    if (!showPayment || !lockResult) return;

    const poll = async () => {
      try {
        const current = lockResultRef.current;
        if (!current) return;
        const result = await tradeApi.getPayment(current.out_trade_no);
        if (result.payment?.status === 1) {
          setShowPayment(false);
          addToast('支付成功！', 'success');
          refreshAll();
        }
      } catch {
        // Not paid yet
      }
    };

    pollTimerRef.current = setInterval(poll, POLL_INTERVAL);
    return () => stopPolling();
  }, [showPayment, lockResult, addToast, refreshAll]);

  const stopPolling = () => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  // Lock: 开团购买 (buy_type=group)
  const handleStartGroup = (product: ProductWithActivity) => {
    if (!isLoggedIn || !product.activity) return;
    stopPolling();
    const outTradeNo = generateOutTradeNo();
    setJoiningTeamId(null);
    lockMutation.mutate(
      {
        user_id: userId!,
        activity_id: product.activity.activity_id,
        goods_id: product.goods_id,
        source: DEFAULT_SOURCE,
        channel: DEFAULT_CHANNEL,
        out_trade_no: outTradeNo,
        buy_type: 'group',
      },
      {
        onSuccess: (data) => {
          setLockResult(data);
          setShowPayment(true);
          setPaymentConfirmClose(false);
          addToast('下单成功', 'success');
          refreshAll();
        },
        onError: (err) => addToast(getErrorMessage(err.code), 'error'),
      },
    );
  };

  // Lock: 直接购买 (buy_type=direct)
  const handleDirectBuy = (product: ProductWithActivity) => {
    if (!isLoggedIn) return;
    stopPolling();
    const outTradeNo = generateOutTradeNo();
    setJoiningTeamId(null);
    lockMutation.mutate(
      {
        user_id: userId!,
        activity_id: 0,
        goods_id: product.goods_id,
        source: DEFAULT_SOURCE,
        channel: DEFAULT_CHANNEL,
        out_trade_no: outTradeNo,
        buy_type: 'direct',
      },
      {
        onSuccess: (data) => {
          setLockResult(data);
          setShowPayment(true);
          setPaymentConfirmClose(false);
          addToast('下单成功', 'success');
        },
        onError: (err) => addToast(getErrorMessage(err.code), 'error'),
      },
    );
  };

  // Join existing team
  const handleJoinTeam = (teamId: string) => {
    if (!isLoggedIn || !selectedProduct?.activity) return;
    stopPolling();
    const outTradeNo = generateOutTradeNo();
    setJoiningTeamId(teamId);
    lockMutation.mutate(
      {
        user_id: userId!,
        activity_id: selectedProduct.activity.activity_id,
        goods_id: selectedProduct.goods_id,
        source: DEFAULT_SOURCE,
        channel: DEFAULT_CHANNEL,
        out_trade_no: outTradeNo,
        team_id: teamId,
        buy_type: 'group',
      },
      {
        onSuccess: (data) => {
          setLockResult(data);
          setShowPayment(true);
          setPaymentConfirmClose(false);
          addToast('加入成功', 'success');
          refreshAll();
        },
        onError: (err) => addToast(getErrorMessage(err.code), 'error'),
      },
    );
  };

  // Not logged in
  if (!isLoggedIn) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-24 text-center">
        <h1 className="heading-serif text-4xl mb-4">
          拼团买，更划算
        </h1>
        <p className="text-[var(--color-text-muted)] mb-8">
          三人成团，享受更优价格
        </p>
        <Button size="lg" onClick={() => navigate('/login')}>
          登录开始
        </Button>
      </div>
    );
  }

  // Loading
  if (productsLoading) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-8">
        <Loading lines={5} />
      </div>
    );
  }

  // Error
  if (productsError) {
    return (
      <ErrorState
        message={
          productsErr ? getErrorMessage((productsErr as { code?: string }).code || '0001') : '加载失败'
        }
        onRetry={() => refetchProducts()}
      />
    );
  }

  // Empty
  if (products.length === 0) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-24 text-center">
        <h2 className="text-xl font-medium text-[var(--color-text-primary)] mb-2">
          暂无商品
        </h2>
        <p className="text-[var(--color-text-muted)]">
          当前没有可购买的商品
        </p>
      </div>
    );
  }

  const teams = teamsData?.items || [];

  return (
    <div className="max-w-2xl mx-auto px-4 py-8">
      {/* Hero heading */}
      <section className="mb-6">
        <h1 className="heading-serif text-3xl mb-1">拼团买，更划算</h1>
        <p className="text-sm text-[var(--color-text-muted)]">
          选择商品，发起或加入拼团，享受优惠价格
        </p>
      </section>

      {/* Product cards */}
      <section className="mb-6 space-y-3">
        {products.map((p) => (
          <div
            key={p.goods_id}
            onClick={() => setSelectedProduct(p)}
            className="cursor-pointer"
          >
            <ProductCard
              product={p}
              isSelected={selectedProduct?.goods_id === p.goods_id}
              onStartGroup={() => handleStartGroup(p)}
              onDirectBuy={() => handleDirectBuy(p)}
              isLockPending={lockMutation.isPending}
            />
          </div>
        ))}
      </section>

      {/* Team list (only for selected product with activity) */}
      {selectedProduct?.activity && (
        <section>
          <div className="flex items-center gap-2 mb-3">
            <h2 className="text-sm font-medium text-[var(--color-text-muted)] uppercase tracking-wide">
              拼团队伍
            </h2>
            <span className="text-xs text-[var(--color-text-muted)]">
              {selectedProduct.goods_name}
            </span>
          </div>
          <TeamList
            teams={teams}
            teamsLoading={teamsLoading}
            isEnabled={true}
            joiningTeamId={joiningTeamId}
            isLockPending={lockMutation.isPending}
            myTeamIds={myTeamIds}
            onJoin={handleJoinTeam}
          />
        </section>
      )}

      {/* Payment Modal */}
      <Modal
        open={showPayment}
        onClose={() => setPaymentConfirmClose(true)}
        title={paymentConfirmClose ? '确认离开' : '扫码支付'}
        maxWidth="sm"
      >
        {paymentConfirmClose ? (
          <div className="flex flex-col items-center gap-4">
            <p className="text-sm text-[var(--color-text-primary)] text-center">
              订单已生成，确定离开支付页面？
            </p>
            <div className="flex gap-3 w-full">
              <Button
                variant="secondary"
                className="flex-1"
                onClick={() => setPaymentConfirmClose(false)}
              >
                继续支付
              </Button>
              <Button
                variant="danger"
                className="flex-1"
                onClick={() => {
                  stopPolling();
                  setShowPayment(false);
                  setPaymentConfirmClose(false);
                  addToast('订单已保留，可在"我的订单"中查看和继续支付', 'info');
                }}
              >
                确定离开
              </Button>
            </div>
          </div>
        ) : lockResult && (
          <div className="flex flex-col items-center gap-4">
            <PriceDisplay
              originalPrice={lockResult.original_price}
              payPrice={lockResult.pay_price}
              deductionPrice={lockResult.deduction_price}
            />

            {/* QR Code */}
            {qrDataUrl ? (
              <div className="flex flex-col items-center gap-2">
                <img
                  src={qrDataUrl}
                  alt="支付二维码"
                  className="w-[280px] h-[280px] rounded-xl border border-[#EAEAEA]"
                />
                <p className="text-xs text-[var(--color-text-muted)] break-all text-center max-w-[280px]">
                  {lockResult.pay_url}
                </p>
              </div>
            ) : (
              <div className="w-[280px] h-[280px] bg-[var(--color-canvas)] border-2 border-[#EAEAEA] rounded-xl flex flex-col items-center justify-center gap-2">
                <div className="w-8 h-8 border-2 border-[var(--color-accent)] border-t-transparent rounded-full animate-spin" />
                <p className="text-sm text-[var(--color-text-muted)]">生成支付二维码...</p>
              </div>
            )}

            {countdown && (
              <p className="text-xs text-[var(--color-text-muted)]">
                支付有效期: {countdown}
              </p>
            )}

            <div className="text-xs text-[var(--color-text-muted)] font-mono">
              <p>订单号: {lockResult.order_id}</p>
              <p>外部单号: {lockResult.out_trade_no}</p>
              {lockResult.team_id && (
                <p className="mt-1 text-[var(--color-warning)]">
                  队伍: {lockResult.team_id}
                </p>
              )}
            </div>

            <div className="flex gap-3 w-full">
              <Button
                variant="secondary"
                className="flex-1"
                onClick={() => setPaymentConfirmClose(true)}
              >
                稍后支付
              </Button>
            </div>

            <p className="text-xs text-[var(--color-text-muted)] text-center">
              请用支付宝扫码支付，支付完成后弹窗自动关闭
            </p>
          </div>
        )}
      </Modal>
    </div>
  );
}
