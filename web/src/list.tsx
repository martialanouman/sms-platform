import { useEffect } from 'react'
import { unreachable } from './api'
import { Refused, Unauthenticated, useSessionClosed } from './session'
import { Button, ErrorState, Toolbar } from './ui'

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

export type Paged = { cursor?: string; trail?: string[] }

export const text = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)

export const paged = (s: Record<string, unknown>): Paged => ({
  cursor: text(s.cursor),
  trail:
    Array.isArray(s.trail) && s.trail.length > 0 && s.trail.every((c) => typeof c === 'string') ? s.trail : undefined,
})

const first = '-'

export function Pager<Search extends Paged>({
  search,
  next,
  go,
}: {
  search: Search
  next?: string | null
  go: (next: Search) => void
}) {
  const trail = search.trail ?? []
  const back = trail.at(-1)
  if (!next && back === undefined) {
    return null
  }
  return (
    <Toolbar>
      {back !== undefined && (
        <Button
          size="sm"
          onClick={() =>
            go({
              ...search,
              cursor: back === first ? undefined : back,
              trail: trail.length > 1 ? trail.slice(0, -1) : undefined,
            })
          }
        >
          Précédent
        </Button>
      )}
      {next && (
        <Button size="sm" onClick={() => go({ ...search, cursor: next, trail: [...trail, search.cursor ?? first] })}>
          Suivant
        </Button>
      )}
    </Toolbar>
  )
}
