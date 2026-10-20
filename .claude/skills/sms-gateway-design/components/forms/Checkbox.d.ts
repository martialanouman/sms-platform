export interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: React.ReactNode;
  /** Second line explaining the consequence (permission grants, policy toggles). */
  description?: React.ReactNode;
  indeterminate?: boolean;
}
export function Checkbox(props: CheckboxProps): JSX.Element;
