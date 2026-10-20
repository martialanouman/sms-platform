export interface SkeletonProps {
  width?: number | string;
  height?: number | string;
  radius?: number | string;
  className?: string;
  style?: React.CSSProperties;
}
export function Skeleton(props: SkeletonProps): JSX.Element;

export interface SkeletonRowsProps {
  rows?: number;
  /** Column widths, mirroring the real table layout. */
  columns?: (number | string)[];
  dense?: boolean;
  className?: string;
}
export function SkeletonRows(props: SkeletonRowsProps): JSX.Element;
