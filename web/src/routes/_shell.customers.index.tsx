import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { atMinute } from '../audit'
import {
  type Customer,
  CustomerForm,
  customersKey,
  GroupFilter,
  GroupName,
  Status,
  statusFilters,
  statusOf,
} from '../customers'
import { gw } from '../gw'
import { Failure, NoResults, type Paged, Pager, paged, text, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, Card, DataTable, EmptyState, Forbidden, LoadingState, needs, PageHeader, Select } from '../ui'

type CustomerStatus = Customer['status']
type CustomersSearch = Paged & { group_id?: string; status?: CustomerStatus }

export const Route = createFileRoute('/_shell/customers/')({
  validateSearch: (s: Record<string, unknown>): CustomersSearch => ({
    ...paged(s),
    group_id: text(s.group_id),
    status: statusOf(s.status),
  }),
  component: Customers,
})

function Customers() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('customers:read')
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
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
  const filtered = search.group_id !== undefined || search.status !== undefined

  return (
    <>
      <PageHeader
        crumbs={['Clients']}
        title="Clients"
        meta={
          readable &&
          'Les clients de l’agrégateur, leur groupe et leur statut. Un client porte un ou plusieurs comptes.'
        }
        actions={
          readable && (
            <Button
              size="sm"
              variant="primary"
              blockedReason={held.includes('customers:write') ? undefined : needs('customers:write')}
              onClick={() => setCreating(true)}
            >
              Nouveau client
            </Button>
          )
        }
      />
      <section className="page">
        {readable ? (
          <>
            <div className="ui-filters">
              <GroupFilter value={search.group_id} onChange={(id) => filter({ group_id: id })} />
              <Select
                label="Statut"
                items={statusFilters}
                value={search.status ?? ''}
                onChange={(status) => filter({ status: status === '' ? undefined : (status as CustomerStatus) })}
              />
            </div>
            {list.isPending ? (
              <LoadingState label="Chargement des clients…" rows={3} />
            ) : list.error ? (
              <Failure
                title="Les clients n’ont pas pu être chargés"
                error={list.error}
                retry={() => void list.refetch()}
              />
            ) : list.data.data.length === 0 ? (
              filtered ? (
                <NoResults clear={() => void navigate({ search: {} })} />
              ) : (
                <EmptyState title="Aucun client">
                  <p>Créez le premier client avec « Nouveau client ».</p>
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
                    noun={['client affiché', 'clients affichés']}
                  />
                }
              >
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
                    { header: 'Statut', cell: (c) => <Status status={c.status} /> },
                    {
                      header: 'Groupe',
                      cell: (c) =>
                        c.group_id === null ? <span className="ui-muted">Aucun</span> : <GroupName id={c.group_id} />,
                    },
                    {
                      header: 'Créé le',
                      cell: (c) => <span className="ui-mono ui-muted">{atMinute(c.created_at)}</span>,
                    },
                  ]}
                />
              </Card>
            )}
            {creating && <CustomerForm onClose={() => setCreating(false)} />}
          </>
        ) : (
          <Forbidden
            title="Accès aux clients non accordé"
            about="Cet écran liste les clients de l’agrégateur, leur groupe et leur statut."
            permission="customers:read"
            grantedBy="Exploitation"
          />
        )}
      </section>
    </>
  )
}
