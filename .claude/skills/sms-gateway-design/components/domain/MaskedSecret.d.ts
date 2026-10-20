export interface MaskedSecretProps {
  /** "Identifiant SMPP" or "Clé API" — exactly two credentials exist per SMPP account. */
  kind?: string;
  /** Last 4 characters, the only part ever displayed. */
  last4?: string;
  status?: 'active' | 'suspended' | 'failed' | 'idle';
  lastUsedAt?: React.ReactNode;
  /** e.g. "en grâce jusqu'au 12/03 08:00". */
  rotationState?: React.ReactNode;
  /** Live bind count — shown so rotation/revocation impact is visible before acting. */
  liveSessions?: number;
  actions?: React.ReactNode;
  className?: string;
}
export function MaskedSecret(props: MaskedSecretProps): JSX.Element;
