/**
 * Primary action control.
 */
export interface ButtonProps {
  /** Outline + tint only — the charter forbids solid fills, destructive included. */
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'dangerGhost' | 'link';
  size?: 'sm' | 'md' | 'lg';
  /** Lucide icon name rendered before the label. */
  icon?: string;
  /** Lucide icon name rendered after the label (chevrons, external-link). */
  iconAfter?: string;
  loading?: boolean;
  disabled?: boolean;
  fullWidth?: boolean;
  type?: 'button' | 'submit' | 'reset';
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  children?: React.ReactNode;
  className?: string;
}
export function Button(props: ButtonProps): JSX.Element;
