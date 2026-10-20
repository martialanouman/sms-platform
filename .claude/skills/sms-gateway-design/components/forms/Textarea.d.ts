export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** Monospace: bulk MSISDN import, JSON payloads, regex conditions. */
  mono?: boolean;
  invalid?: boolean;
  rows?: number;
}
export function Textarea(props: TextareaProps): JSX.Element;
