export interface BadgeProps {
  /** `mt`/`mo` carry the traffic-direction semantics; `script` marks routing-script scope. */
  tone?: 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'mt' | 'mo' | 'script';
  appearance?: 'soft' | 'solid' | 'outline';
  children?: React.ReactNode;
  className?: string;
}
export function Badge(props: BadgeProps): JSX.Element;
