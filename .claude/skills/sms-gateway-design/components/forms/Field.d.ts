export interface FieldProps {
  label?: React.ReactNode;
  htmlFor?: string;
  /** Ajoute « (optionnel) » au libellé. L'inverse — marquer l'obligatoire — n'existe pas. */
  optional?: boolean;
  /** Plain-language consequence of the setting — shown when there is no error. */
  hint?: React.ReactNode;
  error?: React.ReactNode;
  /** Slot next to the label for a permission or scope Badge. */
  badge?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}
export function Field(props: FieldProps): JSX.Element;
