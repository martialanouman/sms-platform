/**
 * MT balance and MO counter are two DIFFERENT objects — never rendered as one figure.
 */
export interface BalanceCardProps {
  /** `mt` = a real, rechargeable, blocking balance. `mo` = a postpaid usage counter that only rises. */
  direction?: 'mt' | 'mo';
  value: number | string;
  unit?: string;
  /** Plain-language explanation — mandatory for `mo`. */
  note?: React.ReactNode;
  /** Meter fill 0–100; use for MT headroom or MO progress to `mo_billing_floor`. */
  meterPct?: number;
  /** `balance_scope` label, always visible ("Pool partagé", "Par compte"). */
  scope?: React.ReactNode;
  /** Account name when the scope is per-account. */
  accountLabel?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}
export function BalanceCard(props: BalanceCardProps): JSX.Element;
