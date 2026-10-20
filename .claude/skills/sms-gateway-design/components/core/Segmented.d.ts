export interface SegmentedItem { value: string; label: React.ReactNode; disabled?: boolean }
export interface SegmentedProps {
  items: (SegmentedItem | string)[];
  value?: string;
  onChange?: (value: string) => void;
  ariaLabel?: string;
  className?: string;
}
export function Segmented(props: SegmentedProps): JSX.Element;
