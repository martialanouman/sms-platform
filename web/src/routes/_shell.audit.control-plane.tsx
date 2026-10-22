import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import type { components } from '../admin.gen'
import { at, Failure, useClosedOn, useOperatorName } from '../audit'
import { gw } from '../gw'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, DataTable, EmptyState, Field, LoadingState, Modal, Toolbar } from '../ui'

type Entry = components['schemas']['AuditEntry']
type Search = { operator?: string; from?: string; to?: string; cursor?: string }

const text = (v: unknown) => (typeof v === 'string' && v !== '' ? v : undefined)

export const Route = createFileRoute('/_shell/audit/control-plane')({
  validateSearch: (s: Record<string, unknown>): Search => ({
    operator: text(s.operator),
    from: text(s.from),
    to: text(s.to),
    cursor: text(s.cursor),
  }),
  component: ControlPlane,
})

const instant = (local: FormDataEntryValue | null) => (text(local) ? new Date(String(local)).toISOString() : undefined)

function ControlPlane() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const held = useShellMe().data?.permissions ?? []
  const name = useOperatorName(held)
  const [changes, setChanges] = useState<string>()
  const log = useQuery({
    queryKey: ['audit', 'control-plane', search],
    queryFn: () =>
      read(
        gw.GET('/admin/audit-log', {
          params: {
            query: { operator: search.operator, from_date: search.from, to_date: search.to, cursor: search.cursor },
          },
        }),
        'audit-log',
      ),
  })
  useClosedOn(log.error)

  const filter = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    void navigate({
      search: { operator: text(form.get('operator')), from: instant(form.get('from')), to: instant(form.get('to')) },
    })
  }

  return (
    <>
      <form className="audit-filters" aria-label="Filtres du journal" onSubmit={filter}>
        <Field label="Opérateur" name="operator" defaultValue={search.operator} />
        <Field label="Depuis" name="from" type="datetime-local" />
        <Field label="Jusqu’à" name="to" type="datetime-local" />
        <Button type="submit" size="sm">
          Filtrer
        </Button>
      </form>
      {log.isPending ? (
        <LoadingState label="Chargement du journal…" rows={3} />
      ) : log.error ? (
        <Failure
          title="Le journal du plan de contrôle n’a pas pu être chargé"
          error={log.error}
          retry={() => void log.refetch()}
        />
      ) : log.data.data.length === 0 ? (
        <EmptyState title="Aucune entrée" />
      ) : (
        <>
          <DataTable<Entry>
            caption="Journal du plan de contrôle"
            rows={log.data.data}
            rowKey={(e) => e.id}
            columns={[
              { header: 'Date', cell: (e) => at(e.at) },
              { header: 'Opérateur', cell: (e) => name(e.operator) },
              { header: 'Opération', cell: (e) => <span className="audit-id">{e.operation_id}</span> },
              { header: 'Cible', cell: (e) => <span className="audit-id">{e.target}</span> },
              { header: 'Statut', cell: (e) => (e.outcome ? `${e.status} ${e.outcome}` : `${e.status}`) },
              {
                header: 'Actions',
                cell: (e) =>
                  e.request_id && (
                    <Button size="sm" onClick={() => setChanges(e.request_id ?? undefined)}>
                      Modifications
                    </Button>
                  ),
              },
            ]}
          />
          {log.data.next_cursor && (
            <Toolbar>
              <Button
                size="sm"
                onClick={() => void navigate({ search: { ...search, cursor: log.data.next_cursor ?? undefined } })}
              >
                Suivant
              </Button>
            </Toolbar>
          )}
        </>
      )}
      {changes && <Changes requestId={changes} onClose={() => setChanges(undefined)} />}
    </>
  )
}

function Changes({ requestId, onClose }: { requestId: string; onClose: () => void }) {
  const list = useQuery({
    queryKey: ['audit', 'changes', requestId],
    queryFn: () =>
      read(
        gw.GET('/admin/audit-log/{request_id}/changes', { params: { path: { request_id: requestId } } }),
        'audit-changes',
      ),
  })
  useClosedOn(list.error)
  return (
    <Modal title={`Modifications de la requête ${requestId}`} wide onClose={onClose} footer={null}>
      {list.isPending ? (
        <LoadingState label="Chargement des modifications…" rows={2} />
      ) : list.error ? (
        <Failure
          title="Les modifications n’ont pas pu être chargées"
          error={list.error}
          retry={() => void list.refetch()}
        />
      ) : list.data.data.length === 0 ? (
        <EmptyState title="Aucune modification enregistrée" />
      ) : (
        list.data.data.map((c) => {
          const columns = [...new Set([...Object.keys(c.before ?? {}), ...Object.keys(c.after ?? {})])]
          return (
            <section className="audit-change" key={`${c.table}${c.pk}${c.at}`}>
              <h3 className="audit-change__table">{c.table}</h3>
              <p className="audit-change__meta">
                <span className="audit-id">{c.pk}</span> · {c.op} · <span className="audit-id">{c.actor}</span> ·{' '}
                {at(c.at)}
              </p>
              <dl className="audit-diff">
                {columns.map((k) => {
                  const before = JSON.stringify(c.before?.[k]) ?? '—'
                  const after = JSON.stringify(c.after?.[k]) ?? '—'
                  return (
                    <div
                      className={before === after ? 'audit-diff__row' : 'audit-diff__row audit-diff__row--changed'}
                      key={k}
                    >
                      <dt>{k}</dt>
                      <dd className="audit-id">{before}</dd>
                      <dd className="audit-id">{after}</dd>
                    </div>
                  )
                })}
              </dl>
            </section>
          )
        })
      )}
    </Modal>
  )
}
