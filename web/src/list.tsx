import { useEffect } from 'react'
import { unreachable } from './api'
import { Refused, Unauthenticated, useSessionClosed } from './session'
import { Button, ErrorState } from './ui'

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
  count,
  noun,
}: {
  search: Search
  next?: string | null
  go: (next: Search) => void
  count: number
  noun: [one: string, many: string]
}) {
  const trail = search.trail ?? []
  const back = trail.at(-1)
  return (
    <div className="ui-pager">
      <span>
        {count} {count === 1 ? noun[0] : noun[1]}
      </span>
      <span className="ui-pager__nav">
        <Button
          size="sm"
          blockedReason={back === undefined ? 'Première page.' : undefined}
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
        <Button
          size="sm"
          blockedReason={next ? undefined : 'Dernière page.'}
          onClick={() => go({ ...search, cursor: next ?? undefined, trail: [...trail, search.cursor ?? first] })}
        >
          Suivant
        </Button>
      </span>
    </div>
  )
}
