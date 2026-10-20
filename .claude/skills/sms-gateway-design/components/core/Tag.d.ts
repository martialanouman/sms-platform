export interface TagProps {
  children?: React.ReactNode;
  /** Monospace variant for sender IDs, MSISDNs and keys. */
  mono?: boolean;
  /** When provided the tag becomes removable (filter chips, sender ID lists). */
  onRemove?: () => void;
  className?: string;
}
export function Tag(props: TagProps): JSX.Element;
