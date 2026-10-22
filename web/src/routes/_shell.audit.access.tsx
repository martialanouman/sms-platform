import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { api } from '../api'
import type { components } from '../api.gen'
import { at, Failure, useClosedOn, useOperatorName } from '../audit'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, DataTable, EmptyState, LoadingState, Toolbar } from '../ui'

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

export const Route = createFileRoute('/_shell/audit/access')({
  validateSearch: (s: Record<string, unknown>): { cursor?: string } => ({
    cursor: typeof s.cursor === 'string' && s.cursor !== '' ? s.cursor : undefined,
  }),
  component: Access,
})

function Access() {
  const { cursor } = Route.useSearch()
  const navigate = Route.useNavigate()
  const name = useOperatorName(useShellMe().data?.permissions ?? [])
  const log = useQuery({
    queryKey: ['audit', 'access', cursor],
    queryFn: () => read(api.GET('/access-log', { params: { query: { cursor } } }), 'access-log'),
  })
  useClosedOn(log.error)

  return log.isPending ? (
    <LoadingState label="Chargement du journal…" rows={3} />
  ) : log.error ? (
    <Failure title="Le journal d’accès n’a pas pu être chargé" error={log.error} retry={() => void log.refetch()} />
  ) : log.data.data.length === 0 ? (
    <EmptyState title="Aucune entrée" />
  ) : (
    <>
      <DataTable<Entry>
        caption="Journal d’accès au tableau de bord"
        rows={log.data.data}
        rowKey={(e) => e.id}
        columns={[
          { header: 'Date', cell: (e) => at(e.at) },
          { header: 'Acteur', cell: (e) => name(e.actor) },
          { header: 'Événement', cell: (e) => events[e.event] },
          { header: 'Cible', cell: (e) => name(e.target) },
          { header: 'Résultat', cell: (e) => (e.outcome === 'success' ? 'Réussi' : 'Échec') },
        ]}
      />
      {log.data.nextCursor && (
        <Toolbar>
          <Button size="sm" onClick={() => void navigate({ search: { cursor: log.data.nextCursor ?? undefined } })}>
            Suivant
          </Button>
        </Toolbar>
      )}
    </>
  )
}
