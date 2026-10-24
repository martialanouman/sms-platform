import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { type Account, AccountContext, AccountForm, accountQuery, accountsKey, EffectiveStatus } from '../accounts'
import { atMinute } from '../audit'
import { customerQuery } from '../customers'
import { gw } from '../gw'
import { Failure, useClosedOn } from '../list'
import { useShellMe } from '../session'
import { Button, Confirmation, Forbidden, LoadingState, needs, PageHeader, Tabs, useToast } from '../ui'

export const Route = createFileRoute('/_shell/accounts/$id')({ component: AccountPage })

const closedFinal = 'Ce compte est fermé définitivement.'

const customersCrumb = (held: string[]) =>
  held.includes('customers:read') ? (
    <Link key="customers" to="/customers">
      Clients
    </Link>
  ) : (
    'Clients'
  )

type Pending = 'rename' | 'suspend' | 'reactivate' | 'close' | 'delete'

function AccountPage() {
  const { id } = Route.useParams()
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('accounts:read')
  const account = useQuery({ ...accountQuery(id), enabled: readable })
  useClosedOn(account.error)

  if (readable && account.data) {
    return <Loaded account={account.data} held={held} />
  }
  return (
    <>
      <PageHeader crumbs={[customersCrumb(held), 'Compte']} title="Compte" />
      <section className="page">
        {!readable ? (
          <Forbidden
            title="Accès aux comptes non accordé"
            about="Cet écran montre un compte d’un client, ses canaux, ses sessions et ses opérations SMPP."
            permission="accounts:read"
            grantedBy="Exploitation"
          />
        ) : account.error ? (
          <Failure
            title="Ce compte n’a pas pu être chargé"
            error={account.error}
            retry={() => void account.refetch()}
          />
        ) : (
          <LoadingState label="Chargement du compte…" rows={3} />
        )}
      </section>
    </>
  )
}

function Loaded({ account, held }: { account: Account; held: string[] }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const notify = useToast()
  const [pending, setPending] = useState<Pending>()
  const close = () => setPending(undefined)
  const done = (text: string) => () => {
    notify(`Compte ${account.name} ${text}.`)
    close()
  }
  const write = held.includes('accounts:write') ? undefined : needs('accounts:write')
  const frozen = write ?? (account.status === 'closed' ? 'Fermé définitivement.' : undefined)
  const path = { params: { path: { id: account.id } } }
  const status = (body: { status: 'active' | 'closed' }) => () => gw.PATCH('/admin/accounts/{id}', { ...path, body })
  const customer = held.includes('customers:read') ? (
    <Link to="/customers/$id" params={{ id: account.customer_id }}>
      {account.customer_name}
    </Link>
  ) : (
    account.customer_name
  )

  return (
    <>
      <PageHeader
        crumbs={[
          customersCrumb(held),
          customer,
          <span key="account" className="ui-mono">
            {account.name}
          </span>,
        ]}
        title={account.name}
        mono
        status={<EffectiveStatus status={account.status} customer={account.customer_status} />}
        meta={
          <>
            Compte de {customer} · créé le <span className="ui-mono">{atMinute(account.created_at)}</span> ·{' '}
            <Button size="sm" blockedReason={frozen} onClick={() => setPending('rename')}>
              Renommer
            </Button>
          </>
        }
        actions={
          <>
            <Button
              size="sm"
              blockedReason={frozen ?? (account.status === 'suspended' ? 'Déjà suspendu.' : undefined)}
              onClick={() => setPending('suspend')}
            >
              Suspendre
            </Button>
            <Button
              size="sm"
              blockedReason={frozen ?? (account.status === 'active' ? 'Déjà actif.' : undefined)}
              onClick={() => setPending('reactivate')}
            >
              Réactiver
            </Button>
            <Button size="sm" variant="danger" blockedReason={frozen} onClick={() => setPending('close')}>
              Clôturer
            </Button>
            <Button size="sm" variant="danger" blockedReason={write} onClick={() => setPending('delete')}>
              Supprimer
            </Button>
          </>
        }
      />
      <section className="page">
        <Tabs
          label="Sections du compte"
          tabs={[
            {
              to: '/accounts/$id/credentials',
              label: 'Identifiants',
              blockedReason: held.includes('credentials:read') ? undefined : needs('credentials:read', 'Cet onglet'),
            },
            { to: '/accounts/$id/channels', label: 'Canaux & opérations' },
            { to: '/accounts/$id/sessions', label: 'Sessions' },
            { to: '/accounts/$id/webhooks', label: 'Webhooks MO/DLR' },
          ]}
        />
        <AccountContext.Provider value={{ account, frozen }}>
          <Outlet />
        </AccountContext.Provider>
      </section>
      {pending === 'rename' && <AccountForm customerId={account.customer_id} account={account} onClose={close} />}
      {pending === 'suspend' && (
        <Confirmation
          title={`Suspendre le compte ${account.name}`}
          action="Suspendre"
          invalidate={accountsKey}
          onClose={close}
          onDone={done('suspendu')}
          refusals={{ 409: closedFinal }}
          call={() => gw.POST('/admin/accounts/{id}/suspend', path)}
        >
          <p>Ses binds ouverts sont déconnectés.</p>
        </Confirmation>
      )}
      {pending === 'reactivate' && (
        <Confirmation
          title={`Réactiver le compte ${account.name}`}
          action="Réactiver"
          invalidate={accountsKey}
          onClose={close}
          onDone={done('réactivé')}
          refusals={{ 422: closedFinal }}
          call={status({ status: 'active' })}
        >
          {account.customer_status === 'suspended' && (
            <p>Son client est suspendu : le compte reste suspendu en pratique.</p>
          )}
        </Confirmation>
      )}
      {pending === 'close' && (
        <Confirmation
          title={`Clôturer le compte ${account.name}`}
          action="Clôturer"
          danger
          invalidate={accountsKey}
          onClose={close}
          onDone={done('fermé')}
          refusals={{ 422: closedFinal }}
          call={status({ status: 'closed' })}
        >
          <p>Définitif : un compte fermé ne se rouvre pas.</p>
        </Confirmation>
      )}
      {pending === 'delete' && (
        <Confirmation
          title={`Supprimer le compte ${account.name}`}
          action="Supprimer"
          danger
          invalidate={accountsKey}
          onClose={close}
          onDone={() => {
            notify(`Compte ${account.name} supprimé.`)
            void queryClient
              .prefetchQuery(customerQuery(account.customer_id))
              .then(() => navigate({ to: '/customers/$id', params: { id: account.customer_id } }))
              .then(() => document.querySelector<HTMLElement>('#contenu h1')?.focus())
          }}
          call={() => gw.DELETE('/admin/accounts/{id}', path)}
        >
          <p>Définitif ; le journal d’audit en garde la trace.</p>
        </Confirmation>
      )}
    </>
  )
}
