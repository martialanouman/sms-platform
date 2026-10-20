export interface Crumb { label: React.ReactNode; href?: string }
export interface TopBarProps {
  title?: React.ReactNode;
  crumbs?: Crumb[];
  /** Status badges rendered next to the title (suspended, script actif). */
  badges?: React.ReactNode;
  actions?: React.ReactNode;
  /** Signed-in operator display name. */
  operator?: string;
  /** Held role — shown because the UI renders from the permission set. */
  role?: string;
  className?: string;
}
export function TopBar(props: TopBarProps): JSX.Element;
