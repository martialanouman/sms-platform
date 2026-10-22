import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useEffect } from 'react'
import { unreachable } from './api'
import type { components } from './api.gen'
import { auditOperatorsQuery } from './queries'
import { Refused, Unauthenticated, useSessionClosed } from './session'
import { Button, Combobox, ErrorState, Field, OperatorCell, Toolbar } from './ui'

const when = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'medium' })

export const at = (iso: string) => when.format(new Date(iso))

export function useOperatorName() {
  const operators = useQuery(auditOperatorsQuery)
  return (id: string | null) => {
    if (!id) {
      return '—'
    }
    const known = operators.data?.find((o) => o.id === id)
    return known ? (
      <OperatorCell name={known.displayName} email={known.email} />
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

export type AuditSearch = { operator?: string; from?: string; to?: string; cursor?: string; trail?: string[] }

const text = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)
const day = (v: unknown) =>
  typeof v === 'string' && new Date(`${v}T00:00`).toLocaleDateString('sv-SE') === v ? v : undefined

export const auditSearch = (s: Record<string, unknown>): AuditSearch => ({
  operator: text(s.operator),
  from: day(s.from),
  to: day(s.to),
  cursor: text(s.cursor),
  trail:
    Array.isArray(s.trail) && s.trail.length > 0 && s.trail.every((c) => typeof c === 'string') ? s.trail : undefined,
})

const first = '-'

export function Pager({
  search,
  next,
  go,
}: {
  search: AuditSearch
  next?: string | null
  go: (next: AuditSearch) => void
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

const today = () => new Date().toLocaleDateString('sv-SE')

export function period(search: AuditSearch) {
  const end = new Date(`${search.to ?? today()}T00:00`)
  end.setDate(end.getDate() + 1)
  return { from: new Date(`${search.from ?? today()}T00:00`).toISOString(), to: end.toISOString() }
}

type Operator = components['schemas']['OperatorRef']

const named = (o: Operator) => (o.status === 'disabled' ? `${o.displayName} (désactivé)` : o.displayName)

const sought = (o: Operator, query: string) =>
  [o.displayName, o.email].some((t) => t.toLocaleLowerCase('fr').includes(query.toLocaleLowerCase('fr')))

export function AuditFilters({ search, onFilter }: { search: AuditSearch; onFilter: (next: AuditSearch) => void }) {
  const operators = useQuery(auditOperatorsQuery).data ?? []
  const filter = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    onFilter({ operator: text(form.get('operator')), from: day(form.get('from')), to: day(form.get('to')) })
  }
  return (
    <form
      key={[operators.length, search.operator, search.from, search.to].join('|')}
      className="audit-filters"
      aria-label="Filtres du journal"
      onSubmit={filter}
    >
      <Combobox
        label="Opérateur"
        name="operator"
        items={operators}
        defaultValue={operators.find((o) => o.id === search.operator) ?? null}
        itemLabel={named}
        itemValue={(o) => o.id}
        itemDetail={(o) => o.email}
        matches={sought}
        empty="Aucun opérateur"
      />
      <Field label="Depuis" name="from" type="date" defaultValue={search.from ?? today()} />
      <Field label="Jusqu’à" name="to" type="date" defaultValue={search.to ?? today()} />
      <Button type="submit">Filtrer</Button>
    </form>
  )
}
