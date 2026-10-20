export interface KeyValueItem { key?: string; label: React.ReactNode; value: React.ReactNode }
export interface KeyValueListProps {
  items: KeyValueItem[];
  /** Monospace values — identifiers, MSISDNs, trace IDs. */
  mono?: boolean;
  /** Stack label above value instead of two columns (narrow detail panels). */
  rows?: boolean;
  className?: string;
}
export function KeyValueList(props: KeyValueListProps): JSX.Element;
