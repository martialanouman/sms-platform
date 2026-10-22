import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { unreachable } from './api'
import { operatorsQuery } from './queries'
import { Refused, Unauthenticated, useSessionClosed } from './session'
import { ErrorState } from './ui'

const when = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'medium' })

export const at = (iso: string) => when.format(new Date(iso))

export function useOperatorName(held: string[]) {
  const operators = useQuery({ ...operatorsQuery, enabled: held.includes('operators:read') })
  return (id: string | null) => {
    if (!id) {
      return '—'
    }
    const name = operators.data?.find((o) => o.id === id)?.displayName
    return name ? (
      <span className="operator-cell">
        <span>{name}</span>
        <span className="operator-cell__email">{id}</span>
      </span>
    ) : (
      <span className="audit-id">{id}</span>
    )
  }
}

export function useClosedOn(error: unknown) {
  const closed = useSessionClosed()
  useEffect(() => {
    if (error instanceof Unauthenticated) {
      closed()
    }
  }, [error, closed])
}

export function Failure({ title, error, retry }: { title: string; error: unknown; retry: () => void }) {
  return (
    <ErrorState as="h2" title={title} onRetry={retry}>
      {error instanceof Refused ? error.message : unreachable}
    </ErrorState>
  )
}
