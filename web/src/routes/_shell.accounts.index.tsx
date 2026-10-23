import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { type Account, accountsKey, channels, EffectiveStatus } from '../accounts'
import { at } from '../audit'
import { customersKey, groupLabel, statusFilters, useGroups } from '../customers'
import { gw } from '../gw'
import { Failure, type Paged, Pager, paged, text, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, Card, DataTable, EmptyState, Forbidden, Icon, LoadingState, PageHeader, Select } from '../ui'

type Status = Account['status']
type AccountsSearch = Paged & { customer_id?: string; group_id?: string; status?: Status }

export const Route = createFileRoute('/_shell/accounts/')({
  validateSearch: (s: Record<string, unknown>): AccountsSearch => ({
    ...paged(s),
    customer_id: text(s.customer_id),
    group_id: text(s.group_id),
    status: s.status === 'active' || s.status === 'suspended' || s.status === 'closed' ? s.status : undefined,
  }),
  component: Accounts,
})

function Accounts() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('accounts:read')
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const groups = useGroups()
  const list = useQuery({
    queryKey: [...accountsKey, search],
    queryFn: () =>
      read(
        gw.GET('/admin/accounts', {
          params: {
            query: {
              customer_id: search.customer_id,
              group_id: search.group_id,
              status: search.status,
              cursor: search.cursor,
            },
          },
        }),
        'accounts',
      ),
    enabled: readable,
  })
  useClosedOn(list.error)
  const filter = (next: Partial<AccountsSearch>) =>
    void navigate({
      search: { customer_id: search.customer_id, group_id: search.group_id, status: search.status, ...next },
    })
  const filtered = search.customer_id !== undefined || search.group_id !== undefined || search.status !== undefined

  return (
    <>
      <PageHeader
        crumbs={['Comptes']}
        title="Comptes"
        meta={
          readable && 'Les comptes SMPP et REST de tous les clients. Un compte se crée depuis la fiche de son client.'
        }
      />
      <section className="page">
        {readable ? (
          <>
            <div className="ui-filters">
              {search.customer_id && (
                <CustomerFilter id={search.customer_id} remove={() => filter({ customer_id: undefined })} />
              )}
              <Select
                label="Groupe"
                items={[
                  { value: '', label: 'Tous' },
                  ...groups.list.map((g) => ({ value: g.id, label: groupLabel(g) })),
                ]}
                value={search.group_id ?? ''}
                onChange={(id) => filter({ group_id: id === '' ? undefined : id })}
                blockedReason={groups.blocked}
              />
              <Select
                label="Statut"
                items={statusFilters}
                value={search.status ?? ''}
                onChange={(status) => filter({ status: status === '' ? undefined : (status as Status) })}
              />
            </div>
            {list.isPending ? (
              <LoadingState label="Chargement des comptes…" rows={3} />
            ) : list.error ? (
              <Failure
                title="Les comptes n’ont pas pu être chargés"
                error={list.error}
                retry={() => void list.refetch()}
              />
            ) : list.data.data.length === 0 ? (
              filtered ? (
                <EmptyState title="Aucun résultat">
                  <p>Élargissez les filtres.</p>
                  <Button size="sm" onClick={() => void navigate({ search: {} })}>
                    Retirer les filtres
                  </Button>
                </EmptyState>
              ) : (
                <EmptyState title="Aucun compte">
                  <p>Un compte se crée depuis la fiche de son client.</p>
                </EmptyState>
              )
            ) : (
              <Card
                flush
                footer={
                  <Pager
                    search={search}
                    next={list.data.next_cursor}
                    go={(next) => void navigate({ search: next })}
                    count={list.data.data.length}
                    noun={['compte affiché', 'comptes affichés']}
                  />
                }
              >
                <DataTable<Account>
                  caption="Comptes"
                  rows={list.data.data}
                  rowKey={(a) => a.id}
                  columns={[
                    {
                      header: 'Nom',
                      cell: (a) => (
                        <Link className="ui-mono" to="/accounts/$id" params={{ id: a.id }}>
                          {a.name}
                        </Link>
                      ),
                    },
                    {
                      header: 'Client',
                      cell: (a) => (
                        <Link to="/customers/$id" params={{ id: a.customer_id }}>
                          {a.customer_name}
                        </Link>
                      ),
                    },
                    {
                      header: 'Statut',
                      cell: (a) => <EffectiveStatus status={a.status} customer={a.customer_status} />,
                    },
                    { header: 'Canaux', cell: (a) => <span className="ui-mono">{channels(a)}</span> },
                    { header: 'Créé le', cell: (a) => <span className="ui-mono ui-muted">{at(a.created_at)}</span> },
                  ]}
                />
              </Card>
            )}
          </>
        ) : (
          <Forbidden
            title="Accès aux comptes non accordé"
            about="Cet écran liste les comptes des clients, leur client et leur statut."
            permission="accounts:read"
            grantedBy="Exploitation"
          />
        )}
      </section>
    </>
  )
}

function CustomerFilter({ id, remove }: { id: string; remove: () => void }) {
  const me = useShellMe()
  const readable = (me.data?.permissions ?? []).includes('customers:read')
  const customer = useQuery({
    queryKey: [...customersKey, id],
    queryFn: () => read(gw.GET('/admin/customers/{id}', { params: { path: { id } } }), 'customer'),
    enabled: readable,
  })
  return (
    <span className="ui-chip">
      Client : {customer.data?.name ?? id}
      <button className="ui-chip__remove" type="button" aria-label="Retirer le filtre Client" onClick={remove}>
        <Icon name="times" />
      </button>
    </span>
  )
}
