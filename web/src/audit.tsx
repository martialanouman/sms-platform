import { useQuery } from '@tanstack/react-query'
import type { ChangeEvent } from 'react'
import type { components } from './api.gen'
import { type Paged, paged, text } from './list'
import { auditOperatorsQuery } from './queries'
import { Button, Combobox, Field, OperatorCell } from './ui'

const when = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'medium' })

export const at = (iso: string) => when.format(new Date(iso))

const minute = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'short', timeStyle: 'short' })

export const atMinute = (iso: string) => minute.format(new Date(iso))

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

export type AuditSearch = Paged & { operator?: string; from?: string; to?: string }

const day = (v: unknown) =>
  typeof v === 'string' && new Date(`${v}T00:00`).toLocaleDateString('sv-SE') === v ? v : undefined

export const auditSearch = (s: Record<string, unknown>): AuditSearch => ({
  ...paged(s),
  operator: text(s.operator),
  from: day(s.from),
  to: day(s.to),
})

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
  const shown = { operator: search.operator, from: search.from, to: search.to }
  const filtered = Object.values(shown).some((v) => v !== undefined)
  const date = (key: 'from' | 'to') => (event: ChangeEvent<HTMLInputElement>) => {
    const value = day(event.currentTarget.value)
    if (value) {
      onFilter({ ...shown, [key]: value })
    }
  }
  return (
    <div
      key={[search.operator, search.from, search.to].join('|')}
      className="audit-filters"
      role="group"
      aria-label="Filtres du journal"
    >
      <Combobox
        key={operators.length}
        label="Opérateur"
        name="operator"
        items={operators}
        defaultValue={operators.find((o) => o.id === search.operator) ?? null}
        itemLabel={named}
        itemValue={(o) => o.id}
        itemDetail={(o) => o.email}
        matches={sought}
        empty="Aucun opérateur"
        onChange={(o) => onFilter({ ...shown, operator: o?.id })}
      />
      <Field label="Du" name="from" type="date" defaultValue={search.from ?? today()} onChange={date('from')} />
      <Field label="Au" name="to" type="date" defaultValue={search.to ?? today()} onChange={date('to')} />
      <Button blockedReason={filtered ? undefined : 'Aucun filtre actif.'} onClick={() => onFilter({})}>
        Retirer les filtres
      </Button>
    </div>
  )
}
