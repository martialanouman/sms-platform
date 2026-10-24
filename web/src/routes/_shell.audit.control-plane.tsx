import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type ReactNode, useState } from 'react'
import type { components } from '../admin.gen'
import { AuditFilters, type AuditSearch, at, auditSearch, period, useOperatorName } from '../audit'
import { gw } from '../gw'
import { Failure, Pager, useClosedOn } from '../list'
import { read } from '../queries'
import { Button, Card, DataTable, EmptyState, KeyValue, LoadingState, Modal, StatusPill, type Tone } from '../ui'

type Entry = components['schemas']['AuditEntry']
export const Route = createFileRoute('/_shell/audit/control-plane')({
  validateSearch: auditSearch,
  component: ControlPlane,
})

function ControlPlane() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const name = useOperatorName()
  const [changes, setChanges] = useState<Entry>()
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
                    <Button size="sm" onClick={() => setChanges(e)}>
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
      {changes?.request_id && (
        <Changes entry={changes} requestId={changes.request_id} onClose={() => setChanges(undefined)} />
      )}
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

type Change = components['schemas']['AuditChange']

const ops: Record<Change['op'], string> = { insert: 'Création', update: 'Modification', delete: 'Suppression' }

const shown = (v: unknown) =>
  v === null || v === undefined ? (
    <span className="ui-muted">vide</span>
  ) : (
    <span className="ui-mono">{typeof v === 'object' ? JSON.stringify(v) : String(v)}</span>
  )

function Diff({ change }: { change: Change }) {
  const before = change.before ?? {}
  const after = change.after ?? {}
  const fields = [...new Set([...Object.keys(before), ...Object.keys(after)])]
  const same = (k: string) => JSON.stringify(before[k]) === JSON.stringify(after[k])
  const kept = fields.filter(same)
  return (
    <section className="audit-change">
      <div className="audit-change__head">
        <span className="ui-tag">{ops[change.op]}</span>
        <h3 className="ui-mono">{change.table}</h3>
        <span className="audit-id">{change.pk}</span>
      </div>
      <DataTable<string>
        caption={`Champs modifiés de ${change.table}`}
        rows={fields.filter((k) => !same(k))}
        rowKey={(k) => k}
        columns={[
          { header: 'Champ', cell: (k) => <span className="ui-mono">{k}</span> },
          { header: 'Avant', cell: (k) => shown(before[k]) },
          { header: 'Après', cell: (k) => shown(after[k]) },
        ]}
      />
      {kept.length > 0 && (
        <details>
          <summary>{kept.length === 1 ? '1 champ inchangé' : `${kept.length} champs inchangés`}</summary>
          <KeyValue items={kept.map((k) => ({ label: k, value: shown(before[k]) }))} />
        </details>
      )}
    </section>
  )
}

function Changes({ entry, requestId, onClose }: { entry: Entry; requestId: string; onClose: () => void }) {
  const name = useOperatorName()
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
    <Modal title={`Modifications de ${entry.operation_id}`} wide onClose={onClose} footer={null}>
      <KeyValue
        items={[
          { label: 'Date', value: <span className="ui-mono">{at(entry.at)}</span> },
          { label: 'Opérateur', value: name(entry.operator) },
          { label: 'Requête', value: <span className="ui-mono">{requestId}</span> },
        ]}
      />
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
        list.data.data.map((c, i) => <Diff change={c} key={`${i}${c.table}${c.pk}`} />)
      )}
      <p className="ui-muted">
        Les secrets et leurs empreintes ne sont jamais journalisés : seuls les champs lisibles apparaissent.
      </p>
      {list.data && (
        <Pager search={position} next={list.data.next_cursor} go={setPosition} count={list.data.data.length} />
      )}
    </Modal>
  )
}
