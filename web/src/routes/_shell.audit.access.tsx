import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { api } from '../api'
import type { components } from '../api.gen'
import { AuditFilters, at, auditSearch, period, useOperatorName } from '../audit'
import { Failure, Pager, useClosedOn } from '../list'
import { read } from '../queries'
import { Card, DataTable, EmptyState, LoadingState, StatusPill } from '../ui'

type Entry = components['schemas']['AccessLogEntry']

const events: Record<Entry['event'], string> = {
  login: 'Connexion',
  logout: 'Déconnexion',
  mfa: 'Second facteur',
  access_link_requested: 'Lien d’accès demandé',
  access_link_used: 'Lien d’accès utilisé',
  factor_enrolled: 'Facteur enrôlé',
  factor_removed: 'Facteur retiré',
  operator_created: 'Opérateur créé',
  operator_disabled: 'Opérateur désactivé',
  operator_enabled: 'Opérateur réactivé',
  roles_changed: 'Rôles modifiés',
  role_created: 'Rôle créé',
  role_updated: 'Rôle modifié',
  role_deleted: 'Rôle supprimé',
  session_closed: 'Session fermée',
  personal_token_issued: 'Jeton personnel délivré',
  session_refused: 'Session refusée',
}

export const Route = createFileRoute('/_shell/audit/access')({ validateSearch: auditSearch, component: Access })

function Access() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const name = useOperatorName()
  const { from, to } = period(search)
  const log = useQuery({
    queryKey: ['audit', 'access', search, from, to],
    queryFn: () =>
      read(
        api.GET('/access-log', {
          params: { query: { actor: search.operator, fromDate: from, toDate: to, cursor: search.cursor } },
        }),
        'access-log',
      ),
  })
  useClosedOn(log.error)

  return (
    <>
      <AuditFilters search={search} onFilter={(next) => void navigate({ search: next })} />
      {log.isPending ? (
        <LoadingState label="Chargement du journal…" rows={3} />
      ) : log.error ? (
        <Failure title="Le journal d’accès n’a pas pu être chargé" error={log.error} retry={() => void log.refetch()} />
      ) : log.data.data.length === 0 ? (
        <EmptyState title="Aucune entrée" />
      ) : (
        <Card
          flush
          footer={
            <Pager
              search={search}
              next={log.data.nextCursor}
              go={(next) => void navigate({ search: next })}
              count={log.data.data.length}
              labels={['Plus récentes', 'Plus anciennes']}
            />
          }
        >
          <DataTable<Entry>
            caption="Journal d’accès au tableau de bord"
            rows={log.data.data}
            rowKey={(e) => e.id}
            columns={[
              { header: 'Date', cell: (e) => <span className="ui-mono">{at(e.at)}</span> },
              { header: 'Acteur', cell: (e) => (e.actor ? name(e.actor) : <span className="ui-muted">inconnu</span>) },
              { header: 'Événement', cell: (e) => events[e.event] },
              { header: 'Cible', cell: (e) => name(e.target) },
              {
                header: 'Résultat',
                cell: (e) =>
                  e.outcome === 'success' ? (
                    <StatusPill tone="ok">Réussi</StatusPill>
                  ) : (
                    <StatusPill tone="warn">Échec</StatusPill>
                  ),
              },
            ]}
          />
        </Card>
      )}
    </>
  )
}
