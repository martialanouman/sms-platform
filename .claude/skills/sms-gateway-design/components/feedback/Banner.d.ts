export interface BannerProps {
  tone?: 'info' | 'success' | 'warning' | 'danger' | 'neutral';
  title?: React.ReactNode;
  children?: React.ReactNode;
  /** Override the tone's default Lucide icon. */
  icon?: string;
  actions?: React.ReactNode;
  className?: string;
}
export function Banner(props: BannerProps): JSX.Element;
