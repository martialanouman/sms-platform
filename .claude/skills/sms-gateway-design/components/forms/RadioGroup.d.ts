export interface RadioOption {
  value: string;
  label: React.ReactNode;
  /** Consequence of the choice, in plain French. */
  description?: React.ReactNode;
  disabled?: boolean;
}
export interface RadioGroupProps {
  name: string;
  value?: string;
  options: RadioOption[];
  row?: boolean;
  disabled?: boolean;
  onChange?: (value: string, e: React.ChangeEvent<HTMLInputElement>) => void;
  className?: string;
}
export function RadioGroup(props: RadioGroupProps): JSX.Element;
