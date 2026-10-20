export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  /** Leading Lucide icon — "search" for filter bars, "hash" for MSISDN fields. */
  icon?: string;
  /** Monospace + tabular: MSISDNs, sender IDs, credit counts, keys. */
  mono?: boolean;
  size?: 'sm' | 'md';
  invalid?: boolean;
}
export function Input(props: InputProps): JSX.Element;
