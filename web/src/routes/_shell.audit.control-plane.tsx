import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type ReactNode, useState } from 'react'
import type { components } from '../admin.gen'
import { AuditFilters, type AuditSearch, at, auditSearch, period, useOperatorName } from '../audit'
import { gw } from '../gw'
import { Failure, Pager, useClosedOn } from '../list'
import { read } from '../queries'
import { Button, Card, DataTable, EmptyState, LoadingState, Modal, StatusPill, type Tone } from '../ui'

type Entry = components['schemas']['AuditEntry']
export const Route = createFileRoute('/_shell/audit/control-plane')({
  validateSearch: auditSearch,
  component: ControlPlane,
})

function ControlPlane() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const name = useOperatorName()
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
          title="Le journal d’administration de la passerelle n’a pas pu être chargé"
          error={log.error}
          retry={() => void log.refetch()}
        />
      ) : log.data.data.length === 0 ? (
        <EmptyState title="Aucune entrée" />
      ) : (
        <Card
          flush
          footer={
            <Pager
              search={search}
              next={log.data.next_cursor}
              go={(next) => void navigate({ search: next })}
              count={log.data.data.length}
              labels={['Plus récentes', 'Plus anciennes']}
            />
          }
        >
          <DataTable<Entry>
            caption="Journal d’administration de la passerelle"
            rows={log.data.data}
            rowKey={(e) => e.id}
            columns={[
              { header: 'Date', cell: (e) => <span className="ui-mono">{at(e.at)}</span> },
              { header: 'Opérateur', cell: (e) => name(e.operator) },
              { header: 'Opération', cell: (e) => <span className="ui-mono">{e.operation_id}</span> },
              { header: 'Cible', cell: (e) => <span className="audit-id">{e.target}</span> },
              { header: 'Résultat', cell: (e) => <Result entry={e} /> },
              {
                header: 'Détail',
                cell: (e) =>
                  e.request_id ? (
                    <Button size="sm" onClick={() => setChanges(e.request_id ?? undefined)}>
                      Modifications
                    </Button>
                  ) : (
                    <span className="ui-muted">aucune</span>
                  ),
              },
            ]}
          />
        </Card>
      )}
      {changes && <Changes requestId={changes} onClose={() => setChanges(undefined)} />}
    </>
  )
}

const outcomes: Record<string, [Tone, string]> = {
  granted: ['ok', 'lecture accordée'],
  unreadable: ['warn', 'illisible'],
  not_found: ['warn', 'introuvable'],
}

function Result({ entry }: { entry: Entry }) {
  const code = <span className="ui-mono">{entry.status}</span>
  const [tone, said]: [Tone, ReactNode] =
    (entry.outcome && outcomes[entry.outcome]) ||
    (entry.status < 400
      ? ['ok', code]
      : entry.status < 500
        ? ['warn', <>{code} refusé</>]
        : ['bad', <>{code} erreur</>])
  return <StatusPill tone={tone}>{said}</StatusPill>
}

function Changes({ requestId, onClose }: { requestId: string; onClose: () => void }) {
  const [position, setPosition] = useState<AuditSearch>({})
  const list = useQuery({
    queryKey: ['audit', 'changes', requestId, position.cursor],
    queryFn: () =>
      read(
        gw.GET('/admin/audit-log/{request_id}/changes', {
          params: { path: { request_id: requestId }, query: { cursor: position.cursor } },
        }),
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
        list.data.data.map((c, i) => {
          const columns = [...new Set([...Object.keys(c.before ?? {}), ...Object.keys(c.after ?? {})])]
          return (
            <section className="audit-change" key={`${i}${c.table}${c.pk}`}>
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
      {list.data && (
        <Pager search={position} next={list.data.next_cursor} go={setPosition} count={list.data.data.length} />
      )}
    </Modal>
  )
}
