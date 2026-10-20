export interface MetricTileProps {
  label: React.ReactNode;
  value: React.ReactNode;
  /** Unit suffix rendered small and muted ("msg/s", "ms", "%"). */
  unit?: React.ReactNode;
  /** `mt`/`mo` colour the number with the traffic-direction accent. */
  tone?: 'default' | 'mt' | 'mo' | 'danger';
  delta?: React.ReactNode;
  deltaDirection?: 'up' | 'down';
  /** Pulsing dot: value is fed by the WS stream. */
  live?: boolean;
  footer?: React.ReactNode;
  size?: 'sm' | 'md';
  className?: string;
}
export function MetricTile(props: MetricTileProps): JSX.Element;
