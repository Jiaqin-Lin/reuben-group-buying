import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import QRCode from 'qrcode';
import { useAuth } from '../../context/AuthContext';
import { useToast } from '../../context/ToastContext';
import { useTrial } from '../../hooks/useTrial';
import { useLockOrder } from '../../hooks/useLockOrder';
import { teamApi } from '../../api/index';
import { tradeApi } from '../../api/trade';
import { orderApi } from '../../api/admin';
import { Button } from '../../components/ui/Button';
import { Card } from '../../components/ui/Card';
import { Badge } from '../../components/ui/Badge';
import { Modal } from '../../components/ui/Modal';
import { PriceDisplay } from '../../components/ui/PriceDisplay';
import { StatusBadge } from '../../components/ui/StatusBadge';
import { Loading } from '../../components/ui/Loading';
import { ErrorState } from '../../components/ui/ErrorState';
import { getErrorMessage } from '../../utils/constants';
import { formatCountdown, generateOutTradeNo } from '../../utils/format';
import type { TrialResult, LockResult, Team } from '../../api/types';

// Default params for demo — must match existing data in DB
const DEFAULT_SOURCE = 'APP';
const DEFAULT_CHANNEL = 'WECHAT';
const DEFAULT_GOODS_ID = 'G_ZJ';
const POLL_INTERVAL = 3000; // 3s
const PAYMENT_TTL = 5 * 60; // 5 min, matches activities.valid_minutes

export function HomePage() {
  const { userId, isLoggedIn } = useAuth();
  const navigate = useNavigate();
  const { addToast } = useToast();
  const queryClient = useQueryClient();
  const trialMutation = useTrial();
  const lockMutation = useLockOrder();

  const [trialResult, setTrialResult] = useState<TrialResult | null>(null);
  const [lockResult, setLockResult] = useState<LockResult | null>(null);
  const [showPayment, setShowPayment] = useState(false);
  const [paymentConfirmClose, setPaymentConfirmClose] = useState(false);
  const [countdown, setCountdown] = useState('');
  const [joiningTeamId, setJoiningTeamId] = useState<string | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [, setTick] = useState(0); // force re-render for real-time countdowns
  const paymentEndRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const tickTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Reusable trial trigger
  const doTrial = useCallback(() => {
    if (!userId) return;
    trialMutation.mutate(
      {
        user_id: userId,
        goods_id: DEFAULT_GOODS_ID,
        source: DEFAULT_SOURCE,
        channel: DEFAULT_CHANNEL,
      },
      {
        onSuccess: (data) => setTrialResult(data),
        onError: (err) => addToast(getErrorMessage(err.code), 'error'),
      },
    );
  }, [userId, trialMutation.mutate, addToast]);

  // Teams list
  const { data: teamsData, isLoading: teamsLoading, refetch: refetchTeams } = useQuery({
    queryKey: ['teams', trialResult?.activity_id],
    queryFn: () => teamApi.listByActivity(trialResult!.activity_id, 1, 20),
    enabled: !!trialResult?.activity_id,
  });

  // User's orders in current activity — for "已加入" indicator
  const { data: myOrders } = useQuery({
    queryKey: ['my-orders', userId],
    queryFn: () => orderApi.listByUser(userId!),
    enabled: isLoggedIn && !!userId,
  });
  const myTeamIds = new Set(
    (myOrders || [])
      .filter((o) => o.activity_id === trialResult?.activity_id && o.status !== 2)
      .map((o) => o.team_id),
  );

  // Load trial on mount or when userId changes
  useEffect(() => {
    if (!isLoggedIn) return;
    doTrial();
  }, [isLoggedIn, userId, doTrial]);

  // Generate QR code when lockResult changes
  useEffect(() => {
    if (!lockResult?.pay_url) {
      setQrDataUrl(null);
      return;
    }
    QRCode.toDataURL(lockResult.pay_url, { width: 280, margin: 1 })
      .then(setQrDataUrl)
      .catch(() => setQrDataUrl(null));
  }, [lockResult?.pay_url]);

  // Countdown timer — driven by showPayment, reads endTime from ref
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

  // Tick timer — force re-render every second for real-time countdowns on team cards
  useEffect(() => {
    tickTimerRef.current = setInterval(() => setTick((t) => t + 1), 1000);
    return () => {
      if (tickTimerRef.current) {
        clearInterval(tickTimerRef.current);
        tickTimerRef.current = null;
      }
    };
  }, []);

  // Poll payment status when modal is open
  const lockResultRef = useRef(lockResult);
  lockResultRef.current = lockResult;

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
          doTrial();
          refetchTeams();
          queryClient.invalidateQueries({ queryKey: ['my-orders', userId] });
        }
      } catch {
        // Not paid yet, keep polling
      }
    };

    pollTimerRef.current = setInterval(poll, POLL_INTERVAL);
    return () => stopPolling();
  }, [showPayment, lockResult, doTrial, addToast, refetchTeams]);

  const stopPolling = () => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const doLock = (teamId?: string) => {
    if (!isLoggedIn) {
      navigate('/login');
      return;
    }
    stopPolling();
    const outTradeNo = generateOutTradeNo();
    setJoiningTeamId(teamId || null);
    lockMutation.mutate(
      {
        user_id: userId!,
        activity_id: trialResult!.activity_id,
        goods_id: DEFAULT_GOODS_ID,
        source: DEFAULT_SOURCE,
        channel: DEFAULT_CHANNEL,
        out_trade_no: outTradeNo,
        team_id: teamId,
      },
      {
        onSuccess: (data) => {
          setLockResult(data);
          setShowPayment(true);
          setPaymentConfirmClose(false);
          addToast('下单成功', 'success');
          refetchTeams();
          queryClient.invalidateQueries({ queryKey: ['my-orders', userId] });
        },
        onError: (err) => {
          addToast(getErrorMessage(err.code), 'error');
        },
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
  if (trialMutation.isPending) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-8">
        <Loading lines={5} />
      </div>
    );
  }

  // Error
  if (trialMutation.isError || !trialResult) {
    return (
      <ErrorState
        message={
          trialMutation.error
            ? getErrorMessage(trialMutation.error.code)
            : '加载失败'
        }
        onRetry={() => doTrial()}
      />
    );
  }

  // Activity not visible
  if (!trialResult.is_visible) {
    return (
      <div className="max-w-2xl mx-auto px-4 py-24 text-center">
        <h2 className="text-xl font-medium text-[var(--color-text-primary)] mb-2">
          暂无可用活动
        </h2>
        <p className="text-[var(--color-text-muted)]">
          当前没有适合您的拼团活动，请稍后再来
        </p>
      </div>
    );
  }

  const teams = teamsData?.items || [];

  return (
    <div className="max-w-2xl mx-auto px-4 py-8">
      {/* Product Hero */}
      <section className="mb-8">
        <div className="flex items-start justify-between mb-3">
          <div>
            <h1 className="text-2xl font-medium text-[var(--color-text-primary)] mb-1">
              {trialResult.goods_name}
            </h1>
            <p className="text-sm text-[var(--color-text-muted)]">
              {trialResult.target_count}人成团
              {trialResult.is_enable ? '，进行中' : '，未开启'}
            </p>
          </div>
          <Badge variant="warning">直降优惠</Badge>
        </div>

        <Card padding="lg" className="mb-6">
          <PriceDisplay
            originalPrice={trialResult.original_price}
            payPrice={trialResult.pay_price}
            deductionPrice={trialResult.deduction_price}
            size="lg"
          />
        </Card>

        {/* Action button */}
        <div className="flex gap-3">
          <Button
            size="lg"
            className="w-full"
            onClick={() => doLock()}
            loading={lockMutation.isPending && !joiningTeamId}
            disabled={!trialResult.is_enable || lockMutation.isPending}
          >
            开团购买
          </Button>
        </div>
      </section>

      {/* Team list */}
      <section>
        <h2 className="text-base font-medium text-[var(--color-text-primary)] mb-4">
          进行中的拼团
        </h2>
        {teamsLoading ? (
          <div className="flex flex-col gap-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Card key={i} padding="md" className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-full bg-[var(--color-canvas)] animate-pulse flex-shrink-0" />
                <div className="flex-1 space-y-2">
                  <div className="h-4 bg-[var(--color-canvas)] rounded animate-pulse w-24" />
                  <div className="h-3 bg-[var(--color-canvas)] rounded animate-pulse w-40" />
                </div>
                <div className="h-8 w-16 bg-[var(--color-canvas)] rounded animate-pulse flex-shrink-0" />
              </Card>
            ))}
          </div>
        ) : teams.length === 0 ? (
          <Card padding="sm" className="text-center py-12">
            <p className="text-sm text-[var(--color-text-muted)]">
              来做第一个开团的人
            </p>
          </Card>
        ) : (
          <div className="flex flex-col gap-3">
            {teams.map((team: Team) => {
              const remainingSeconds = Math.max(
                0,
                Math.floor((new Date(team.valid_end).getTime() - Date.now()) / 1000),
              );
              const progress = team.target_count > 0
                ? Math.round((team.lock_count / team.target_count) * 100)
                : 0;
              const isJoining = joiningTeamId === team.team_id && lockMutation.isPending;
              const isMyTeam = myTeamIds.has(team.team_id);

              return (
                <Card key={team.team_id} padding="md" className="flex items-center gap-4">
                  {/* Progress ring */}
                  <div className="relative w-12 h-12 flex-shrink-0">
                    <svg className="w-12 h-12 -rotate-90" viewBox="0 0 36 36">
                      <circle
                        cx="18" cy="18" r="15"
                        fill="none"
                        stroke="#EAEAEA"
                        strokeWidth="3"
                      />
                      <circle
                        cx="18" cy="18" r="15"
                        fill="none"
                        stroke="var(--color-accent)"
                        strokeWidth="3"
                        strokeDasharray={`${progress * 0.94} 94`}
                        strokeLinecap="round"
                      />
                    </svg>
                    <span className="absolute inset-0 flex items-center justify-center text-xs font-mono font-medium text-[var(--color-text-primary)]">
                      {team.lock_count}/{team.target_count}
                    </span>
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-0.5">
                      <span className="text-sm font-medium text-[var(--color-text-primary)] truncate font-mono">
                        {team.team_id}
                      </span>
                      <StatusBadge type="team" status={team.status} />
                      {isMyTeam && <Badge variant="info">已加入</Badge>}
                    </div>
                    <div className="flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                      <span>{team.lock_count}人已参与</span>
                      <span>剩余 {formatCountdown(remainingSeconds)}</span>
                    </div>
                  </div>

                  {/* Join button */}
                  <Button
                    variant="secondary"
                    size="sm"
                    className="flex-shrink-0"
                    onClick={() => doLock(team.team_id)}
                    loading={isJoining}
                    disabled={!trialResult.is_enable || team.lock_count >= team.target_count}
                  >
                    {team.lock_count >= team.target_count ? '已满' : '加入'}
                  </Button>
                </Card>
              );
            })}
          </div>
        )}
      </section>

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

            {/* QR Code — 280x280 */}
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
              <p className="mt-1 text-[var(--color-warning)]">
                队伍: {lockResult.team_id}
              </p>
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
