import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { type Account, accountsKey, effective } from '../accounts'
import { at } from '../audit'
import { customersKey, groupLabel, useGroups } from '../customers'
import { gw } from '../gw'
import { Failure, type Paged, Pager, paged, text, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, DataTable, EmptyState, Forbidden, LoadingState, PageHeader, Select, Toolbar } from '../ui'

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

const statusFilters = [
  { value: '', label: 'Tous' },
  { value: 'active', label: 'Actifs' },
  { value: 'suspended', label: 'Suspendus' },
  { value: 'closed', label: 'Clos' },
]

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

  return (
    <>
      <PageHeader crumbs={['Comptes']} title="Comptes" />
      {readable ? (
        <>
          <Toolbar
            filters={
              <>
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
              </>
            }
          />
          <section className="page">
            {list.isPending ? (
              <LoadingState label="Chargement des comptes…" rows={3} />
            ) : list.error ? (
              <Failure
                title="Les comptes n’ont pas pu être chargés"
                error={list.error}
                retry={() => void list.refetch()}
              />
            ) : list.data.data.length === 0 ? (
              <EmptyState title="Aucun compte" />
            ) : (
              <>
                <DataTable<Account>
                  caption="Comptes"
                  rows={list.data.data}
                  rowKey={(a) => a.id}
                  columns={[
                    {
                      header: 'Nom',
                      cell: (a) => (
                        <Link to="/accounts/$id" params={{ id: a.id }}>
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
                    { header: 'Statut', cell: (a) => effective(a.status, a.customer_status) },
                    { header: 'Créé le', cell: (a) => at(a.created_at) },
                  ]}
                />
                <Pager search={search} next={list.data.next_cursor} go={(next) => void navigate({ search: next })} />
              </>
            )}
          </section>
        </>
      ) : (
        <section className="page">
          <Forbidden
            title="Accès aux comptes non accordé"
            about="Cet écran liste les comptes des clients, leur client et leur statut."
            permission="accounts:read"
            grantedBy="Exploitation"
          />
        </section>
      )}
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
    <span className="ui-filter">
      Client : {customer.data?.name ?? id}
      <Button size="sm" aria-label="Retirer le filtre Client" onClick={remove}>
        Retirer
      </Button>
    </span>
  )
}
