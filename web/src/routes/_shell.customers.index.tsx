import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { at } from '../audit'
import { type Customer, CustomerForm, customerStatus, customersKey, groupLabel, useGroups } from '../customers'
import { gw } from '../gw'
import { Failure, type Paged, Pager, paged, text, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, DataTable, EmptyState, Forbidden, LoadingState, needs, PageHeader, Select, Toolbar } from '../ui'

type Status = Customer['status']
type CustomersSearch = Paged & { group_id?: string; status?: Status }

export const Route = createFileRoute('/_shell/customers/')({
  validateSearch: (s: Record<string, unknown>): CustomersSearch => ({
    ...paged(s),
    group_id: text(s.group_id),
    status: s.status === 'active' || s.status === 'suspended' || s.status === 'closed' ? s.status : undefined,
  }),
  component: Customers,
})

const statusFilters = [
  { value: '', label: 'Tous' },
  { value: 'active', label: 'Actifs' },
  { value: 'suspended', label: 'Suspendus' },
  { value: 'closed', label: 'Clos' },
]

function Customers() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('customers:read')
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const groups = useGroups()
  const list = useQuery({
    queryKey: [...customersKey, search],
    queryFn: () =>
      read(
        gw.GET('/admin/customers', {
          params: { query: { group_id: search.group_id, status: search.status, cursor: search.cursor } },
        }),
        'customers',
      ),
    enabled: readable,
  })
  useClosedOn(list.error)
  const [creating, setCreating] = useState(false)
  const filter = (next: Partial<CustomersSearch>) =>
    void navigate({ search: { group_id: search.group_id, status: search.status, ...next } })

  return (
    <>
      <PageHeader crumbs={['Clients']} title="Clients" />
      {readable ? (
        <>
          <Toolbar
            filters={
              <>
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
          >
            <Button
              size="sm"
              variant="primary"
              blockedReason={held.includes('customers:write') ? undefined : needs('customers:write')}
              onClick={() => setCreating(true)}
            >
              Nouveau client
            </Button>
          </Toolbar>
          <section className="page">
            {list.isPending ? (
              <LoadingState label="Chargement des clients…" rows={3} />
            ) : list.error ? (
              <Failure
                title="Les clients n’ont pas pu être chargés"
                error={list.error}
                retry={() => void list.refetch()}
              />
            ) : list.data.data.length === 0 ? (
              <EmptyState title="Aucun client" />
            ) : (
              <>
                <DataTable<Customer>
                  caption="Clients"
                  rows={list.data.data}
                  rowKey={(c) => c.id}
                  columns={[
                    {
                      header: 'Nom',
                      cell: (c) => (
                        <Link to="/customers/$id" params={{ id: c.id }}>
                          {c.name}
                        </Link>
                      ),
                    },
                    { header: 'Groupe', cell: (c) => groups.name(c.group_id) },
                    { header: 'Statut', cell: (c) => customerStatus[c.status] },
                    { header: 'Créé le', cell: (c) => at(c.created_at) },
                  ]}
                />
                <Pager search={search} next={list.data.next_cursor} go={(next) => void navigate({ search: next })} />
              </>
            )}
            {creating && <CustomerForm onClose={() => setCreating(false)} />}
          </section>
        </>
      ) : (
        <section className="page">
          <Forbidden
            title="Accès aux clients non accordé"
            about="Cet écran liste les clients de l’agrégateur, leur groupe et leur statut."
            permission="customers:read"
            grantedBy="Exploitation"
          />
        </section>
      )}
    </>
  )
}
