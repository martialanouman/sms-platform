export interface EmptyStateProps {
  icon?: string;
  title?: React.ReactNode;
  /** Why it is empty, and what to do — never a bare "Aucun résultat". */
  children?: React.ReactNode;
  action?: React.ReactNode;
  /** Tighter padding for use inside a card or table body. */
  inline?: boolean;
  /** Drops the dashed frame — use when already inside a bordered container. */
  bare?: boolean;
  className?: string;
}
export function EmptyState(props: EmptyStateProps): JSX.Element;
