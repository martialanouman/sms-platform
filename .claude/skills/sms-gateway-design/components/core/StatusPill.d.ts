export interface StatusPillProps {
  /**
   * `up|reconnecting|down` (+ delivery/account states) render as a coloured DOT with a
   * mono label. `closed|open|half_open` render as a tinted PILL — the charter keeps the
   * two dimensions visually distinct because they demand opposite actions (charte §06).
   */
  state: 'up' | 'reconnecting' | 'down' | 'closed' | 'open' | 'half_open' | 'active' | 'suspended'
    | 'delivered' | 'failed' | 'pending' | 'throttled' | 'expired' | 'restricted' | 'idle' | 'unbound' | 'unknown' | 'connected';
  /** Overrides the machine label. */
  label?: string;
  /** Mono suffix for dot form — counts, timings ("3/4 binds", "p99 212 ms"). */
  meta?: string;
  /** Pulses the dot; reserved for WS-fed values. */
  live?: boolean;
  /** Plain-language note beside the breaker pill ("circuit ouvert · pause"). */
  note?: React.ReactNode;
  className?: string;
}
export function StatusPill(props: StatusPillProps): JSX.Element;
