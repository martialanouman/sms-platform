export interface CardProps {
  title?: React.ReactNode;
  subtitle?: React.ReactNode;
  /** Right-aligned header controls. */
  actions?: React.ReactNode;
  footer?: React.ReactNode;
  /** Removes body padding — use when the body is a table. */
  flush?: boolean;
  flat?: boolean;
  sunken?: boolean;
  children?: React.ReactNode;
  className?: string;
}
export function Card(props: CardProps): JSX.Element;
