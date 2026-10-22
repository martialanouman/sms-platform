import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import type { components } from '../admin.gen'
import { AuditFilters, at, auditSearch, Failure, period, useClosedOn, useOperatorName } from '../audit'
import { gw } from '../gw'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, DataTable, EmptyState, LoadingState, Modal, Toolbar } from '../ui'

type Entry = components['schemas']['AuditEntry']
export const Route = createFileRoute('/_shell/audit/control-plane')({
  validateSearch: auditSearch,
  component: ControlPlane,
})

function ControlPlane() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const held = useShellMe().data?.permissions ?? []
  const name = useOperatorName(held)
  const [changes, setChanges] = useState<string>()
  const { from, to } = period(search)
  const log = useQuery({
    queryKey: ['audit', 'control-plane', search, from, to],
    queryFn: () =>
      read(
        gw.GET('/admin/audit-log', {
          params: { query: { operator: search.operator, from_date: from, to_date: to, cursor: search.cursor } },
        }),
        'audit-log',
      ),
  })
  useClosedOn(log.error)

  return (
    <>
      <AuditFilters search={search} onFilter={(next) => void navigate({ search: next })} />
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
