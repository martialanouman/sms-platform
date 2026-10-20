export interface PaginationProps {
  /** Human range string ("1 – 100"). */
  range?: React.ReactNode;
  /** Total or approximate total; omit when the cursor API cannot count. */
  total?: React.ReactNode;
  pageSize?: number;
  onPageSizeChange?: (size: number) => void;
  hasPrev?: boolean;
  hasNext?: boolean;
  onPrev?: () => void;
  onNext?: () => void;
  /** Freshness or governance note ("Fraîcheur ~15 s"). */
  note?: React.ReactNode;
  className?: string;
}
export function Pagination(props: PaginationProps): JSX.Element;
