export interface NavItem {
  id: string;
  label: React.ReactNode;
  /** Optional — the charter's nav is label-only; only pass a glyph from the closed set. */
  icon?: string;
  count?: React.ReactNode;
}
export interface NavGroup { label: React.ReactNode; items: NavItem[] }
export interface SideNavProps {
  /** Two-letter monogram in the teal gradient tile (the brand mark from the charter). */
  monogram?: string;
  wordmark?: string;
  /** Environment pill ("PROD", "STAGING"). */
  env?: string;
  groups: NavGroup[];
  activeId?: string;
  onNavigate?: (id: string) => void;
  className?: string;
}
export function SideNav(props: SideNavProps): JSX.Element;
