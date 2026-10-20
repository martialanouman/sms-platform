export interface SelectOption { value: string; label: string; disabled?: boolean }
export interface SelectProps extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'children'> {
  options: (SelectOption | string)[];
  size?: 'sm' | 'md';
  /** Renders an empty leading option — use for optional filters ("Tous les groupes"). */
  placeholder?: string;
}
export function Select(props: SelectProps): JSX.Element;
