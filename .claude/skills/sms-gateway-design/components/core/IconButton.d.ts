export interface IconButtonProps {
  /** Lucide icon name. */
  icon: string;
  /** Required accessible label (also the tooltip). */
  label: string;
  variant?: 'ghost' | 'secondary' | 'danger';
  size?: 'sm' | 'md';
  disabled?: boolean;
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  className?: string;
}
export function IconButton(props: IconButtonProps): JSX.Element;
