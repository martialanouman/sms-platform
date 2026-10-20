export interface ToastProps {
  severity?: 'info' | 'success' | 'warning' | 'critical';
  title?: React.ReactNode;
  children?: React.ReactNode;
  /** Origin of the notification: `alertmanager`, `bff_evaluator`, `billing_alert_stream`. */
  source?: string;
  onClose?: () => void;
  className?: string;
}
export function Toast(props: ToastProps): JSX.Element;
export interface ToastStackProps { children?: React.ReactNode; className?: string }
export function ToastStack(props: ToastStackProps): JSX.Element;
