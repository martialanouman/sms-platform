import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { type Account, AccountForm, accountsKey, effective } from '../accounts'
import { at } from '../audit'
import { ageFields, type Customer, CustomerForm, customerStatus, customersKey, useGroups } from '../customers'
import { gw } from '../gw'
import { Failure, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import { Button, Confirmation, DataTable, Forbidden, LoadingState, PageHeader, Toolbar } from '../ui'

export const Route = createFileRoute('/_shell/customers/$id')({ component: CustomerPage })

const closedFinal = 'Ce client est clos définitivement.'

type Pending = 'edit' | 'suspend' | 'reactivate' | 'close' | 'delete'

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

  return (
    <>
      <PageHeader title={customer.data?.name ?? 'Client'} />
      <section className="page">
        <Link to="/customers">← Clients</Link>
        {!readable ? (
          <Forbidden
            title="Accès aux clients non accordé"
            about="Cet écran montre un client de l’agrégateur, son groupe, son statut et ses âges maximaux."
            permission="customers:read"
            grantedBy="Exploitation"
          />
        ) : customer.isPending ? (
          <LoadingState label="Chargement du client…" rows={3} />
        ) : customer.error ? (
          <Failure
            title="Ce client n’a pas pu être chargé"
            error={customer.error}
            retry={() => void customer.refetch()}
          />
        ) : (
          <Loaded customer={customer.data} writes={held.includes('customers:write')} />
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
  const identity = useId()
  const write = writes ? undefined : 'Cette action demande customers:write.'
  const closed = customer.status === 'closed' ? 'Clos définitivement.' : undefined
  const status = (body: { status: 'active' | 'closed' }) => () =>
    gw.PATCH('/admin/customers/{id}', { params: { path: { id: customer.id } }, body })

  return (
    <>
      <Toolbar>
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
      </Toolbar>
      <section aria-labelledby={identity}>
        <h2 id={identity}>Identité</h2>
        <dl>
          <dt>Statut</dt>
          <dd>{customerStatus[customer.status]}</dd>
          <dt>Groupe</dt>
          <dd>
            {customer.group_id ? (
              <Link to="/customers" search={{ group_id: customer.group_id }}>
                {groups.name(customer.group_id)}
              </Link>
            ) : (
              'Aucun'
            )}
          </dd>
          {ageFields.map((f) => {
            const age = customer.max_age_override[f.key]
            return [
              <dt key={`${f.key}-label`}>{f.label}</dt>,
              <dd key={f.key}>{age === null ? 'Plafond de la plateforme' : `${age} s`}</dd>,
            ]
          })}
          <dt>Créé le</dt>
          <dd>{at(customer.created_at)}</dd>
          <dt>Modifié le</dt>
          <dd>{at(customer.updated_at)}</dd>
        </dl>
      </section>
      <Accounts customer={customer} />
      {pending === 'edit' && <CustomerForm customer={customer} onClose={close} />}
      {pending === 'suspend' && (
        <Confirmation
          title={`Suspendre le client ${customer.name}`}
          action="Suspendre"
          invalidate={customersKey}
          onClose={close}
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
          refusals={{ 422: closedFinal }}
          call={status({ status: 'closed' })}
        >
          <p>Définitif : ses comptes non clos sont clos, et un client clos ne se rouvre pas.</p>
        </Confirmation>
      )}
      {pending === 'delete' && (
        <Confirmation
          title={`Supprimer le client ${customer.name}`}
          action="Supprimer"
          danger
          invalidate={customersKey}
          onClose={close}
          onDone={() => void navigate({ to: '/customers' })}
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
        ` ${count(closed.length, '1 compte clos le reste', '{n} comptes clos le restent')} : ${names(closed)}.`}
    </p>
  )
}

function Accounts({ customer }: { customer: Customer }) {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('accounts:read')
  const heading = useId()
  const [creating, setCreating] = useState(false)
  const accounts = useQuery({
    queryKey: [...accountsKey, { customer_id: customer.id }],
    queryFn: () => read(gw.GET('/admin/accounts', { params: { query: { customer_id: customer.id } } }), 'accounts'),
    enabled: readable,
  })

  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>Comptes</h2>
      {!readable ? (
        <p>Les comptes demandent accounts:read.</p>
      ) : (
        <>
          <Toolbar>
            <Link to="/accounts" search={{ customer_id: customer.id }}>
              Tous les comptes
            </Link>
            <Button
              size="sm"
              blockedReason={
                held.includes('accounts:write')
                  ? customer.status === 'closed'
                    ? 'Clos définitivement.'
                    : undefined
                  : 'Cette action demande accounts:write.'
              }
              onClick={() => setCreating(true)}
            >
              Nouveau compte
            </Button>
          </Toolbar>
          {accounts.isPending ? (
            <LoadingState label="Chargement des comptes…" rows={2} />
          ) : accounts.error ? (
            <Failure
              title="Les comptes n’ont pas pu être chargés"
              error={accounts.error}
              retry={() => void accounts.refetch()}
            />
          ) : accounts.data.data.length === 0 ? (
            <p>Aucun compte.</p>
          ) : (
            <DataTable<Account>
              caption="Comptes du client"
              rows={accounts.data.data}
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
                { header: 'Statut', cell: (a) => effective(a.status, customer.status) },
                { header: 'Créé le', cell: (a) => at(a.created_at) },
              ]}
            />
          )}
          {creating && <AccountForm customerId={customer.id} onClose={() => setCreating(false)} />}
        </>
      )}
    </section>
  )
}
