export interface ErrorStateProps {
  title?: React.ReactNode;
  /** What still works — the charter requires stating that local data stays visible. */
  children?: React.ReactNode;
  /** Mono request trace line: "GET /api/connectors · 504 · req_8f2c…". */
  request?: React.ReactNode;
  /** Retry control. */
  action?: React.ReactNode;
  className?: string;
}
export function ErrorState(props: ErrorStateProps): JSX.Element;
