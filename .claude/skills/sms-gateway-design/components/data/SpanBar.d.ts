export interface SpanBarProps {
  name: React.ReactNode;
  /** Nesting level in the waterfall. */
  depth?: number;
  /** Bar offset as a percentage of the total trace duration. */
  startPct?: number;
  widthPct?: number;
  /** Pre-formatted duration ("12 ms"). */
  duration?: React.ReactNode;
  state?: 'ok' | 'slow' | 'failed' | 'skipped';
  icon?: string;
  className?: string;
}
export function SpanBar(props: SpanBarProps): JSX.Element;
