import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
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
  const filtered =
    search.operator !== undefined || [search.from, search.to].some((d) => d !== undefined && d !== today())
  return (
    <fieldset className="audit-filters" aria-label="Filtres du journal">
      <Combobox
        label="Opérateur"
        items={operators}
        value={operators.find((o) => o.id === search.operator) ?? null}
        itemLabel={named}
        itemValue={(o) => o.id}
        itemDetail={(o) => o.email}
        matches={sought}
        empty="Aucun opérateur"
        onChange={(o) => onFilter({ ...shown, operator: o?.id })}
      />
      <Day label="Du" value={search.from} onApply={(from) => onFilter({ ...shown, from })} />
      <Day label="Au" value={search.to} onApply={(to) => onFilter({ ...shown, to })} />
      <Button blockedReason={filtered ? undefined : 'Aucun filtre actif.'} onClick={() => onFilter({})}>
        Retirer les filtres
      </Button>
    </fieldset>
  )
}

function Day({ label, value, onApply }: { label: string; value?: string; onApply: (day: string) => void }) {
  const shown = value ?? today()
  const [draft, setDraft] = useState(shown)
  useEffect(() => setDraft(shown), [shown])
  const apply = () => {
    const chosen = day(draft)
    if (chosen && chosen !== shown) {
      onApply(chosen)
    }
  }
  return (
    <Field
      label={label}
      type="date"
      value={draft}
      onChange={(e) => setDraft(e.currentTarget.value)}
      onBlur={apply}
      onKeyDown={(e) => e.key === 'Enter' && apply()}
    />
  )
}
