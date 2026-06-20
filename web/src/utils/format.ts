/** Format price string (from decimal string like "80.00") to display */
export function formatPrice(price: string): string {
  const n = parseFloat(price);
  if (isNaN(n)) return price;
  return n % 1 === 0 ? `¥${n.toFixed(0)}` : `¥${n.toFixed(2)}`;
}

/** Format datetime string for display */
export function formatTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** Calculate remaining time in human-readable format */
export function formatCountdown(seconds: number): string {
  if (seconds <= 0) return '已结束';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}时${m}分${s}秒`;
  if (m > 0) return `${m}分${s}秒`;
  return `${s}秒`;
}

/** Generate a random out_trade_no (12-digit numeric, matching system expectation) */
export function generateOutTradeNo(): string {
  const ts = Date.now().toString().slice(-8);
  const rand = Math.floor(Math.random() * 10000)
    .toString()
    .padStart(4, '0');
  return ts + rand;
}
