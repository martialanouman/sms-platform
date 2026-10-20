export interface DataColumn<T = any> {
  key: string;
  header: React.ReactNode;
  /** Right-aligned monospace — counts, rates, latencies, credits. */
  numeric?: boolean;
  /** Monospace left-aligned — MSISDNs, IDs, bind identifiers. */
  mono?: boolean;
  muted?: boolean;
  width?: number | string;
  sortable?: boolean;
  render?: (row: T, index: number) => React.ReactNode;
}
/**
 * The console's primary data surface.
 */
export interface DataTableProps<T = any> {
  columns: DataColumn<T>[];
  rows: T[];
  rowKey?: (row: T, index: number) => string | number;
  /** 30px rows instead of 36px — session monitor and CDR results. */
  dense?: boolean;
  selectedKey?: string | number;
  onRowClick?: (row: T, key: string | number) => void;
  sort?: { key: string; dir: 'asc' | 'desc' };
  onSortChange?: (key: string) => void;
  /** Rendered inside the body when `rows` is empty — pass an EmptyState. */
  empty?: React.ReactNode;
  className?: string;
}
export function DataTable<T>(props: DataTableProps<T>): JSX.Element;
