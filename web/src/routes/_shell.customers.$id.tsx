import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { type Account, AccountForm, accountsKey, channels, EffectiveStatus } from '../accounts'
import { at } from '../audit'
import { ageFields, type Customer, CustomerForm, customersKey, Status, useGroups } from '../customers'
import { gw } from '../gw'
import { Failure, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import {
  Button,
  Card,
  Confirmation,
  DataTable,
  Forbidden,
  KeyValue,
  LoadingState,
  needs,
  PageHeader,
  useToast,
} from '../ui'

export const Route = createFileRoute('/_shell/customers/$id')({ component: CustomerPage })

const closedFinal = 'Ce client est fermé définitivement.'

type Pending = 'edit' | 'suspend' | 'reactivate' | 'close' | 'delete'

const customersCrumb = (
  <Link key="customers" to="/customers">
    Clients
  </Link>
)

function CustomerPage() {
  const { id } = Route.useParams()
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('customers:read')
  const customer = useQuery({
    queryKey: [...customersKey, id],
    queryFn: () => read(gw.GET('/admin/customers/{id}', { params: { path: { id } } }), 'customer'),
    enabled: readable,
  })
  useClosedOn(customer.error)

  if (readable && customer.data) {
    return <Loaded customer={customer.data} writes={held.includes('customers:write')} />
  }
  return (
    <>
      <PageHeader crumbs={[customersCrumb, 'Client']} title="Client" />
      <section className="page">
        {!readable ? (
          <Forbidden
            title="Accès aux clients non accordé"
            about="Cet écran montre un client de l’agrégateur, son groupe, son statut et ses âges maximaux."
            permission="customers:read"
            grantedBy="Exploitation"
          />
        ) : customer.error ? (
          <Failure
            title="Ce client n’a pas pu être chargé"
            error={customer.error}
            retry={() => void customer.refetch()}
          />
        ) : (
          <LoadingState label="Chargement du client…" rows={3} />
        )}
      </section>
    </>
  )
}

function Loaded({ customer, writes }: { customer: Customer; writes: boolean }) {
  const groups = useGroups()
  const navigate = useNavigate()
  const [pending, setPending] = useState<Pending>()
  const close = () => setPending(undefined)
  const notify = useToast()
  const done = (text: string) => () => {
    notify(`Client ${customer.name} ${text}.`)
    close()
  }
  const write = writes ? undefined : needs('customers:write')
  const closed = customer.status === 'closed' ? 'Fermé définitivement.' : undefined
  const status = (body: { status: 'active' | 'closed' }) => () =>
    gw.PATCH('/admin/customers/{id}', { params: { path: { id: customer.id } }, body })

  return (
    <>
      <PageHeader
        crumbs={[customersCrumb, customer.name]}
        title={customer.name}
        status={<Status status={customer.status} />}
        meta={
          <>
            Créé le <span className="ui-mono">{at(customer.created_at)}</span> · modifié le{' '}
            <span className="ui-mono">{at(customer.updated_at)}</span>
          </>
        }
        actions={
          <>
            <Button size="sm" blockedReason={write ?? closed} onClick={() => setPending('edit')}>
              Modifier
            </Button>
            <Button
              size="sm"
              blockedReason={write ?? closed ?? (customer.status === 'suspended' ? 'Déjà suspendu.' : undefined)}
              onClick={() => setPending('suspend')}
            >
              Suspendre
            </Button>
            <Button
              size="sm"
              blockedReason={write ?? closed ?? (customer.status === 'active' ? 'Déjà actif.' : undefined)}
              onClick={() => setPending('reactivate')}
            >
              Réactiver
            </Button>
            <Button size="sm" variant="danger" blockedReason={write ?? closed} onClick={() => setPending('close')}>
              Clôturer
            </Button>
            <Button size="sm" variant="danger" blockedReason={write} onClick={() => setPending('delete')}>
              Supprimer
            </Button>
          </>
        }
      />
      <section className="page">
        <div className="ui-cards">
          <Card title="Identité">
            <KeyValue
              items={[
                {
                  label: 'Groupe',
                  value: customer.group_id ? (
                    <Link to="/customers" search={{ group_id: customer.group_id }}>
                      {groups.name(customer.group_id)}
                    </Link>
                  ) : (
                    <span className="ui-muted">Aucun</span>
                  ),
                },
                { label: 'Statut', value: <Status status={customer.status} /> },
              ]}
            />
          </Card>
          <Card
            title="Âges maximaux"
            aside={<span className="ui-why">abaissent seulement le plafond de la plateforme</span>}
          >
            <KeyValue
              items={ageFields.map((f) => {
                const age = customer.max_age_override[f.key]
                return {
                  label: f.label,
                  value:
                    age === null ? (
                      <>
                        <span className="ui-muted">Plafond de la plateforme</span>{' '}
                        <span className="ui-mono ui-muted">{f.max.toLocaleString('fr')} s</span>
                      </>
                    ) : (
                      <span className="ui-mono">{age.toLocaleString('fr')} s</span>
                    ),
                }
              })}
            />
          </Card>
        </div>
        <Accounts customer={customer} />
      </section>
      {pending === 'edit' && <CustomerForm customer={customer} onClose={close} />}
      {pending === 'suspend' && (
        <Confirmation
          title={`Suspendre le client ${customer.name}`}
          action="Suspendre"
          invalidate={customersKey}
          onClose={close}
          onDone={done('suspendu')}
          refusals={{ 409: closedFinal }}
          call={() => gw.POST('/admin/customers/{id}/suspend', { params: { path: { id: customer.id } } })}
        >
          <Impact id={customer.id} />
        </Confirmation>
      )}
      {pending === 'reactivate' && (
        <Confirmation
          title={`Réactiver le client ${customer.name}`}
          action="Réactiver"
          invalidate={customersKey}
          onClose={close}
          onDone={done('réactivé')}
          refusals={{ 422: closedFinal }}
          call={status({ status: 'active' })}
        >
          <p>Ses comptes restent suspendus ; réactivez-les un à un.</p>
        </Confirmation>
      )}
      {pending === 'close' && (
        <Confirmation
          title={`Clôturer le client ${customer.name}`}
          action="Clôturer"
          danger
          invalidate={customersKey}
          onClose={close}
          onDone={done('fermé')}
          refusals={{ 422: closedFinal }}
          call={status({ status: 'closed' })}
        >
          <p>Définitif : ses comptes non fermés sont fermés, et un client fermé ne se rouvre pas.</p>
        </Confirmation>
      )}
      {pending === 'delete' && (
        <Confirmation
          title={`Supprimer le client ${customer.name}`}
          action="Supprimer"
          danger
          invalidate={customersKey}
          onClose={close}
          onDone={() => {
            notify(`Client ${customer.name} supprimé.`)
            void navigate({ to: '/customers' })
          }}
          refusals={{ 409: 'Ce client a des comptes : suspendez-le ou clôturez-le.' }}
          call={() => gw.DELETE('/admin/customers/{id}', { params: { path: { id: customer.id } } })}
        >
          <p>Un client ne se supprime que sans compte ni trafic.</p>
        </Confirmation>
      )}
    </>
  )
}

const count = (n: number, one: string, many: string) => (n === 1 ? one : many.replace('{n}', String(n)))

function Impact({ id }: { id: string }) {
  const impact = useQuery({
    queryKey: [...customersKey, id, 'suspension-impact'],
    queryFn: () =>
      read(gw.GET('/admin/customers/{id}/suspension-impact', { params: { path: { id } } }), 'suspension-impact'),
  })
  if (impact.isPending) {
    return <p>Calcul de l’impact…</p>
  }
  if (impact.error) {
    return <p>Impact indisponible.</p>
  }
  const { accounts_to_suspend: suspend, accounts_closed: closed } = impact.data
  const names = (accounts: { name: string }[]) => accounts.map((a) => a.name).join(', ')
  return (
    <p>
      {suspend.length === 0
        ? 'Aucun compte à suspendre.'
        : `${count(suspend.length, '1 compte sera suspendu', '{n} comptes seront suspendus')} : ${names(suspend)}.`}
      {closed.length > 0 &&
        ` ${count(closed.length, '1 compte fermé le reste', '{n} comptes fermés le restent')} : ${names(closed)}.`}
    </p>
  )
}

function Accounts({ customer }: { customer: Customer }) {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('accounts:read')
  const [creating, setCreating] = useState(false)
  const accounts = useQuery({
    queryKey: [...accountsKey, { customer_id: customer.id }],
    queryFn: () => read(gw.GET('/admin/accounts', { params: { query: { customer_id: customer.id } } }), 'accounts'),
    enabled: readable,
  })
  const rows = readable ? (accounts.data?.data ?? []) : []

  return (
    <Card
      title="Comptes"
      flush={rows.length > 0}
      aside={
        readable && (
          <span className="ui-card__actions">
            <Link to="/accounts" search={{ customer_id: customer.id }}>
              Tous les comptes
            </Link>
            <Button
              size="sm"
              variant="primary"
              blockedReason={
                held.includes('accounts:write')
                  ? customer.status === 'closed'
                    ? 'Fermé définitivement.'
                    : undefined
                  : needs('accounts:write')
              }
              onClick={() => setCreating(true)}
            >
              Nouveau compte
            </Button>
          </span>
        )
      }
    >
      {!readable ? (
        needs('accounts:read', 'La liste des comptes')
      ) : accounts.isPending ? (
        <LoadingState label="Chargement des comptes…" rows={2} />
      ) : accounts.error ? (
        <Failure
          title="Les comptes n’ont pas pu être chargés"
          error={accounts.error}
          retry={() => void accounts.refetch()}
        />
      ) : rows.length === 0 ? (
        <p className="ui-muted">Aucun compte.</p>
      ) : (
        <DataTable<Account>
          caption="Comptes du client"
          rows={rows}
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
            { header: 'Statut', cell: (a) => <EffectiveStatus status={a.status} customer={customer.status} /> },
            { header: 'Canaux', cell: (a) => <span className="ui-mono">{channels(a)}</span> },
            { header: 'Créé le', cell: (a) => <span className="ui-mono ui-muted">{at(a.created_at)}</span> },
          ]}
        />
      )}
      {creating && <AccountForm customerId={customer.id} onClose={() => setCreating(false)} />}
    </Card>
  )
}
