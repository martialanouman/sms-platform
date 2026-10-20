/**
 * The charter's complete functional glyph set. Names outside it render NOTHING —
 * decorative pictograms and emoji are forbidden (charte §07).
 * Available: dot, square, diamond, circle, warning, bang, info, plus, minus, times,
 * check, chevron-up|down|left|right, arrow-up, arrow-down, refresh, search, ban,
 * ellipsis, ellipsis-vertical, bell.
 */
export interface IconProps {
  name: string;
  /** 14px default (controls, rows); 16px in headers and empty states; never below 12. */
  size?: number;
  strokeWidth?: number;
  className?: string;
  style?: React.CSSProperties;
  /** Accessible label; without it the glyph is aria-hidden. */
  title?: string;
}
export function Icon(props: IconProps): JSX.Element | null;

export interface DotProps {
  tone?: 'up' | 'degraded' | 'down' | 'restricted' | 'accent' | 'info' | 'idle';
  size?: number;
  /** Pulses at 1.8s — reserved for values arriving over the WS stream. */
  live?: boolean;
  className?: string;
}
export function Dot(props: DotProps): JSX.Element;
