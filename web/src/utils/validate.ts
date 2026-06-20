export type FieldErrors = Record<string, string>;

/** Check value is non-empty (string or number). */
export function required(value: string | number | undefined | null, label: string): string | null {
  if (value === undefined || value === null || value === '') return `${label}不能为空`;
  return null;
}

/** Check string has minimum length. */
export function minLength(value: string, min: number, label: string): string | null {
  if (value.length < min) return `${label}至少${min}个字符`;
  return null;
}

/** Check value is a positive integer. */
export function isPositiveInt(value: number | string, label: string): string | null {
  const n = typeof value === 'string' ? Number(value) : value;
  if (!Number.isFinite(n) || n <= 0 || !Number.isInteger(n)) return `${label}必须为正整数`;
  return null;
}

/** Check number >= min. */
export function minValue(value: number | string, min: number, label: string): string | null {
  const n = typeof value === 'string' ? Number(value) : value;
  if (!Number.isFinite(n) || n < min) return `${label}不能小于${min}`;
  return null;
}

/** Check number is valid (finite, not NaN). */
export function isValidNumber(value: number | string, label: string): string | null {
  const n = typeof value === 'string' ? Number(value) : value;
  if (!Number.isFinite(n)) return `${label}必须为有效数字`;
  return null;
}

/** Check end date is after start date (both must be non-empty). */
export function dateRange(start: string, end: string): string | null {
  if (!start || !end) return null;
  if (new Date(end).getTime() <= new Date(start).getTime()) {
    return '结束时间必须晚于开始时间';
  }
  return null;
}

/** Check expression format matches plan type. */
export function expressionFormat(value: string, planType: string): string | null {
  if (!value) return null;
  switch (planType) {
    case 'ZJ': // 直减: plain number like 20
      if (!/^\d+(\.\d{1,2})?$/.test(value)) return '直减表达式需为数字，如 20';
      break;
    case 'MJ': // 满减: threshold-amount like 100-20
      if (!/^\d+(\.\d{1,2})?-\d+(\.\d{1,2})?$/.test(value)) return '满减表达式需为 满减额-减额，如 100-20';
      break;
    case 'ZK': // 折扣: decimal like 0.8
      if (!/^0\.\d{1,2}$/.test(value)) return '折扣表达式需为 0.x 格式，如 0.8';
      break;
    case 'N':   // 固定价: price like 9.99
      if (!/^\d+(\.\d{1,2})?$/.test(value)) return '固定价表达式需为数字，如 9.99';
      break;
  }
  return null;
}

/** Check price format (digits with optional 1-2 decimal places). */
export function priceFormat(value: string, label: string): string | null {
  if (!/^\d+(\.\d{1,2})?$/.test(value)) return `${label}格式不正确，如 19.99`;
  return null;
}

/**
 * Run a set of validation rules and return an errors object.
 * Only includes fields that have errors (non-null results).
 */
export function validateForm(rules: Array<{ field: string; check: () => string | null }>): FieldErrors {
  const errors: FieldErrors = {};
  for (const { field, check } of rules) {
    const err = check();
    if (err) errors[field] = err;
  }
  return errors;
}

/** Remove a field from errors (e.g. when user starts correcting it). */
export function clearError(prev: FieldErrors, field: string): FieldErrors {
  const next = { ...prev };
  delete next[field];
  return next;
}
