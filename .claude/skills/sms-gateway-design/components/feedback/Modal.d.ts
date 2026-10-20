export interface ModalProps {
  open?: boolean;
  title?: React.ReactNode;
  /** Leading node in the header (a StatusPill or Icon). */
  icon?: React.ReactNode;
  /** 800px instead of 520px — credential creation, simulation results. */
  wide?: boolean;
  onClose?: () => void;
  /** Right-aligned footer actions; destructive confirm on the right. */
  footer?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}
export function Modal(props: ModalProps): JSX.Element;
