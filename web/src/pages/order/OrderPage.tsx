import { useState, useEffect, useRef } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import QRCode from 'qrcode';
import { useAuth } from '../../context/AuthContext';
import { useToast } from '../../context/ToastContext';
import { useRefund } from '../../hooks/useRefund';
import { orderApi } from '../../api/admin';
import { tradeApi } from '../../api/trade';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import { Modal } from '../../components/ui/Modal';
import { PriceDisplay } from '../../components/ui/PriceDisplay';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { ErrorState } from '../../components/ui/ErrorState';
import { PageLoading } from '../../components/ui/Loading';
import { formatCountdown, formatTime } from '../../utils/format';
import { getErrorMessage } from '../../utils/constants';
import type { Payment } from '../../api/types';

const POLL_INTERVAL = 3000;
const PAYMENT_TTL = 5 * 60; // 5 min, matches activities.valid_minutes

export function OrderPage() {
  const { outTradeNo } = useParams<{ outTradeNo: string }>();
  const { userId } = useAuth();
  const { addToast } = useToast();
  const queryClient = useQueryClient();
  const refundMutation = useRefund();
  const [refunded, setRefunded] = useState(false);
  const [showPayment, setShowPayment] = useState(false);
  const [payment, setPayment] = useState<Payment | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [countdown, setCountdown] = useState('');
  const [paymentConfirmClose, setPaymentConfirmClose] = useState(false);
  const paymentEndRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const { data: order, isLoading, error } = useQuery({
    queryKey: ['order', outTradeNo],
    queryFn: () => orderApi.getByOutTradeNo(outTradeNo!),
    enabled: !!outTradeNo,
  });

  // Load payment info and generate QR for "continue payment"
  const loadPayment = async () => {
    if (!outTradeNo) return;
    try {
      const result = await tradeApi.getPayment(outTradeNo);
      setPayment(result.payment);
      const payUrl = result.payment?.pay_url || result.payment?.qr_code_url;
      if (payUrl) {
        const dataUrl = await QRCode.toDataURL(payUrl, { width: 280, margin: 1 });
        setQrDataUrl(dataUrl);
      } else {
        setQrDataUrl(null);
      }
    } catch {
      addToast('获取支付信息失败', 'error');
    }
  };

  // Poll payment status when modal is open
  useEffect(() => {
    if (!showPayment || !outTradeNo) return;

    const poll = async () => {
      try {
        const result = await tradeApi.getPayment(outTradeNo);
        setPayment(result.payment);
        if (result.payment?.status === 1) {
          setShowPayment(false);
          stopPolling();
          addToast('支付成功！', 'success');
          queryClient.invalidateQueries({ queryKey: ['order', outTradeNo] });
        }
      } catch {
        // keep polling
      }
    };

    pollTimerRef.current = setInterval(poll, POLL_INTERVAL);
    return () => stopPolling();
  }, [showPayment, outTradeNo]);

  // Countdown timer — based on order creation, NOT fresh each time modal opens
  useEffect(() => {
    if (!showPayment || !order) {
      setCountdown('');
      return;
    }
    const orderCreatedAt = new Date(order.created_at).getTime();
    const expiresAt = orderCreatedAt + PAYMENT_TTL * 1000;
    paymentEndRef.current = expiresAt;

    const update = () => {
      const left = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
      setCountdown(formatCountdown(left));
      if (left <= 0 && countdownTimerRef.current) {
        clearInterval(countdownTimerRef.current);
      }
    };
    update();

    countdownTimerRef.current = setInterval(update, 1000);

    return () => {
      if (countdownTimerRef.current) {
        clearInterval(countdownTimerRef.current);
        countdownTimerRef.current = null;
      }
    };
  }, [showPayment, order]);

  const stopPolling = () => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const handleContinuePay = async () => {
    if (!order) return;
    if (Date.now() > new Date(order.created_at).getTime() + PAYMENT_TTL * 1000) {
      addToast('支付已过期，请取消订单后重新下单', 'error');
      return;
    }
    await loadPayment();
    setShowPayment(true);
  };

  const handleRefund = () => {
    if (!order || !userId) return;
    refundMutation.mutate(
      {
        user_id: userId,
        out_trade_no: order.out_trade_no,
      },
      {
        onSuccess: (data) => {
          addToast(`退款成功 (${data.refund_type})`, 'success');
          setRefunded(true);
        },
        onError: (err) => {
          addToast(getErrorMessage(err.code), 'error');
        },
      },
    );
  };

  if (isLoading) return <PageLoading />;
  if (error || !order) {
    return (
      <ErrorState
        message="订单不存在或加载失败"
        onRetry={() => window.location.reload()}
      />
    );
  }

  const effectiveStatus = refunded ? 2 : order.status;
  const orderCreatedAt = new Date(order.created_at).getTime();
  const paymentExpiresAt = orderCreatedAt + PAYMENT_TTL * 1000;
  const isPaymentExpired = Date.now() > paymentExpiresAt;

  return (
    <div className="max-w-2xl mx-auto px-4 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-medium text-[var(--color-text-primary)]">
          订单详情
        </h1>
        <Link
          to="/orders"
          className="text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors no-underline"
        >
          全部订单
        </Link>
      </div>

      <Card padding="lg" className="mb-4">
        <div className="flex items-center justify-between mb-4">
          <StatusBadge type="order" status={effectiveStatus} />
          <span className="text-xs text-[var(--color-text-muted)] font-mono">
            {order.order_id}
          </span>
        </div>

        <PriceDisplay
          originalPrice={order.original_price}
          payPrice={order.pay_price}
          deductionPrice={order.deduction_price}
          size="md"
          className="mb-4"
        />

        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <span className="text-[var(--color-text-muted)]">外部单号</span>
            <p className="font-mono text-xs text-[var(--color-text-primary)] mt-0.5 break-all">
              {order.out_trade_no}
            </p>
          </div>
          <div>
            <span className="text-[var(--color-text-muted)]">拼团队伍</span>
            <p className="font-mono text-xs text-[var(--color-text-primary)] mt-0.5">
              {order.team_id}
            </p>
          </div>
          <div>
            <span className="text-[var(--color-text-muted)]">活动 ID</span>
            <p className="text-xs text-[var(--color-text-primary)] mt-0.5">
              {order.activity_id}
            </p>
          </div>
          <div>
            <span className="text-[var(--color-text-muted)]">创建时间</span>
            <p className="text-xs text-[var(--color-text-primary)] mt-0.5">
              {formatTime(order.created_at)}
            </p>
          </div>
          {order.out_trade_time && (
            <div>
              <span className="text-[var(--color-text-muted)]">支付时间</span>
              <p className="text-xs text-[var(--color-text-primary)] mt-0.5">
                {formatTime(order.out_trade_time)}
              </p>
            </div>
          )}
        </div>
      </Card>

      {/* Actions */}
      {effectiveStatus === 0 && (isPaymentExpired ? (
        <Card padding="md" className="text-center">
          <Badge variant="warning">支付已过期</Badge>
          <p className="text-sm text-[var(--color-text-muted)] mt-2">
            支付有效期已过，请取消订单后重新下单
          </p>
        </Card>
      ) : (
        <div className="flex gap-3">
          <Button
            variant="secondary"
            className="flex-1"
            onClick={handleContinuePay}
          >
            继续支付
          </Button>
          <Button
            variant="danger"
            className="flex-1"
            onClick={handleRefund}
            loading={refundMutation.isPending}
          >
            取消订单
          </Button>
        </div>
      ))}
      {effectiveStatus === 1 && (
        <div className="flex gap-3">
          <Button
            variant="danger"
            className="flex-1"
            onClick={handleRefund}
            loading={refundMutation.isPending}
          >
            申请退款
          </Button>
        </div>
      )}
      {effectiveStatus === 2 && (
        <Card padding="md" className="text-center">
          <Badge variant="error">已退款</Badge>
          <p className="text-sm text-[var(--color-text-muted)] mt-2">
            该订单已退款
          </p>
        </Card>
      )}

      {/* Payment Modal (continue pay) */}
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
                  setQrDataUrl(null);
                  setPaymentConfirmClose(false);
                }}
              >
                确定离开
              </Button>
            </div>
          </div>
        ) : (
        <div className="flex flex-col items-center gap-4">
          <PriceDisplay
            originalPrice={order.original_price}
            payPrice={order.pay_price}
            deductionPrice={order.deduction_price}
          />

          {/* QR Code — 280x280 */}
          {qrDataUrl ? (
            <div className="flex flex-col items-center gap-2">
              <img
                src={qrDataUrl}
                alt="支付二维码"
                className="w-[280px] h-[280px] rounded-xl border border-[#EAEAEA]"
              />
              {payment?.pay_url && (
                <p className="text-xs text-[var(--color-text-muted)] break-all text-center max-w-[280px]">
                  {payment.pay_url}
                </p>
              )}
            </div>
          ) : (
            <div className="w-[280px] h-[280px] bg-[var(--color-canvas)] border-2 border-[#EAEAEA] rounded-xl flex flex-col items-center justify-center gap-2">
              <div className="w-8 h-8 border-2 border-[var(--color-accent)] border-t-transparent rounded-full animate-spin" />
              <p className="text-sm text-[var(--color-text-muted)]">生成支付二维码...</p>
            </div>
          )}

          <div className="text-xs text-[var(--color-text-muted)] font-mono">
            <p>订单号: {order.order_id}</p>
            <p>外部单号: {order.out_trade_no}</p>
          </div>

          {countdown && (
            <p className="text-xs text-[var(--color-text-muted)] text-center">
              支付有效期: {countdown}
            </p>
          )}

          <div className="flex gap-3 w-full">
            <Button
              variant="secondary"
              className="flex-1"
              onClick={() => {
                stopPolling();
                setShowPayment(false);
                setQrDataUrl(null);
              }}
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
