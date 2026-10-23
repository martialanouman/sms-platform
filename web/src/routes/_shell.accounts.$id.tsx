import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { type ComponentProps, type FormEvent, type ReactNode, useId, useState } from 'react'
import { type Account, AccountForm, accountsKey, effective } from '../accounts'
import { at } from '../audit'
import { Credentials } from '../credentials'
import { gw } from '../gw'
import { Failure, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import {
  Button,
  Choice,
  Choices,
  Confirmation,
  Field,
  Forbidden,
  LoadingState,
  Modal,
  PageHeader,
  Toolbar,
  useSubmission,
} from '../ui'

export const Route = createFileRoute('/_shell/accounts/$id')({ component: AccountPage })

const closedFinal = 'Ce compte est clos définitivement.'
const bindTypes = ['transmitter', 'receiver', 'transceiver'] as const
const onOff = (on: boolean) => (on ? 'Activé' : 'Désactivé')
const checked = (...pairs: [boolean, string][]) => pairs.filter(([on]) => on).map(([, value]) => value)

type AccountPath = { params: { path: { id: string } } }

type Pending = 'rename' | 'channels' | 'sessions' | 'ops' | 'suspend' | 'reactivate' | 'close' | 'delete'

function AccountPage() {
  const { id } = Route.useParams()
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('accounts:read')
  const account = useQuery({
    queryKey: [...accountsKey, id],
    queryFn: () => read(gw.GET('/admin/accounts/{id}', { params: { path: { id } } }), 'account'),
    enabled: readable,
  })
  useClosedOn(account.error)

  return (
    <>
      <PageHeader title={account.data?.name ?? 'Compte'} />
      <section className="page">
        {!readable ? (
          <Forbidden
            title="Accès aux comptes non accordé"
            about="Cet écran montre un compte d’un client, ses canaux, ses sessions et ses opérations SMPP."
            permission="accounts:read"
            grantedBy="Exploitation"
          />
        ) : account.isPending ? (
          <LoadingState label="Chargement du compte…" rows={3} />
        ) : account.error ? (
          <Failure
            title="Ce compte n’a pas pu être chargé"
            error={account.error}
            retry={() => void account.refetch()}
          />
        ) : (
          <Loaded account={account.data} writes={held.includes('accounts:write')} />
        )}
      </section>
    </>
  )
}

function Block({ title, edit, children }: { title: string; edit: ReactNode; children: ReactNode }) {
  const heading = useId()
  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>{title}</h2>
      {edit}
      <dl>{children}</dl>
    </section>
  )
}

function Loaded({ account, writes }: { account: Account; writes: boolean }) {
  const navigate = useNavigate()
  const [pending, setPending] = useState<Pending>()
  const close = () => setPending(undefined)
  const write = writes ? undefined : 'Cette action demande accounts:write.'
  const frozen = write ?? (account.status === 'closed' ? 'Clos définitivement.' : undefined)
  const path = { params: { path: { id: account.id } } }
  const status = (body: { status: 'active' | 'closed' }) => () => gw.PATCH('/admin/accounts/{id}', { ...path, body })
  const edit = (next: Pending) => (
    <Button size="sm" blockedReason={frozen} onClick={() => setPending(next)}>
      Modifier
    </Button>
  )

  return (
    <>
      <Link to="/customers/$id" params={{ id: account.customer_id }}>
        ← {account.customer_name}
      </Link>
      <Toolbar>
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
      </Toolbar>
      <Block title="Identité" edit={edit('rename')}>
        <dt>Statut</dt>
        <dd>{effective(account.status, account.customer_status)}</dd>
        <dt>Créé le</dt>
        <dd>{at(account.created_at)}</dd>
        <dt>Modifié le</dt>
        <dd>{at(account.updated_at)}</dd>
      </Block>
      <Block title="Canaux" edit={edit('channels')}>
        <dt>SMPP</dt>
        <dd>{onOff(account.smpp_enabled)}</dd>
        <dt>REST</dt>
        <dd>{onOff(account.rest_enabled)}</dd>
      </Block>
      <Block title="Sessions" edit={edit('sessions')}>
        <dt>Sessions simultanées</dt>
        <dd>{account.max_sessions}</dd>
        <dt>Types de bind</dt>
        <dd>{account.allowed_bind_types.join(', ')}</dd>
      </Block>
      <Block title="Opérations SMPP" edit={edit('ops')}>
        <dt>query_sm</dt>
        <dd>{onOff(account.query_sm_enabled)}</dd>
        <dt>cancel_sm</dt>
        <dd>{onOff(account.cancel_sm_enabled)}</dd>
      </Block>
      <Credentials account={account} />
      {pending === 'rename' && <AccountForm customerId={account.customer_id} account={account} onClose={close} />}
      {pending === 'channels' && (
        <Settings
          title={`Canaux du compte ${account.name}`}
          label="Canaux"
          initial={checked([account.smpp_enabled, 'smpp'], [account.rest_enabled, 'rest'])}
          options={[
            { value: 'smpp', label: 'SMPP' },
            { value: 'rest', label: 'REST' },
          ]}
          last="Au moins un canal reste activé."
          onClose={close}
          save={(on) => () =>
            gw.PATCH('/admin/accounts/{id}/channels', {
              ...path,
              body: { smpp_enabled: on.includes('smpp'), rest_enabled: on.includes('rest') },
            })
          }
        />
      )}
      {pending === 'sessions' && <SessionsForm account={account} path={path} onClose={close} />}
      {pending === 'ops' && (
        <Settings
          title={`Opérations SMPP du compte ${account.name}`}
          label="Opérations SMPP"
          initial={checked([account.query_sm_enabled, 'query_sm'], [account.cancel_sm_enabled, 'cancel_sm'])}
          options={[
            { value: 'query_sm', label: 'query_sm' },
            { value: 'cancel_sm', label: 'cancel_sm' },
          ]}
          about="Les drapeaux se lisent au bind : les binds ouverts du compte sont déconnectés."
          onClose={close}
          save={(on) => {
            const query = on.includes('query_sm')
            const cancel = on.includes('cancel_sm')
            if (query === account.query_sm_enabled && cancel === account.cancel_sm_enabled) {
              return undefined
            }
            return () =>
              gw.PATCH('/admin/accounts/{id}/smpp-ops', {
                ...path,
                body: {
                  ...(query !== account.query_sm_enabled && { query_sm_enabled: query }),
                  ...(cancel !== account.cancel_sm_enabled && { cancel_sm_enabled: cancel }),
                },
              })
          }}
        />
      )}
      {pending === 'suspend' && (
        <Confirmation
          title={`Suspendre le compte ${account.name}`}
          action="Suspendre"
          invalidate={accountsKey}
          onClose={close}
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
          refusals={{ 422: closedFinal }}
          call={status({ status: 'closed' })}
        >
          <p>Définitif : un compte clos ne se rouvre pas.</p>
        </Confirmation>
      )}
      {pending === 'delete' && (
        <Confirmation
          title={`Supprimer le compte ${account.name}`}
          action="Supprimer"
          danger
          invalidate={accountsKey}
          onClose={close}
          onDone={() => void navigate({ to: '/customers/$id', params: { id: account.customer_id } })}
          call={() => gw.DELETE('/admin/accounts/{id}', path)}
        >
          <p>Définitif ; le journal d’audit en garde la trace.</p>
        </Confirmation>
      )}
    </>
  )
}

function Settings({
  title,
  label,
  initial,
  options,
  last,
  about,
  save,
  onClose,
  children,
}: {
  title: string
  label: string
  initial: string[]
  options: { value: string; label: string }[]
  last?: string
  about?: string
  save: (on: string[]) => ComponentProps<typeof Confirmation>['call'] | undefined
  onClose: () => void
  children?: ReactNode
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [on, setOn] = useState(initial)
  const form = useId()
  const alone = last !== undefined && on.length === 1

  async function confirm(event: FormEvent) {
    event.preventDefault()
    const call = save(on)
    if (call === undefined) {
      onClose()
      return
    }
    const outcome = await submit(call)
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: accountsKey })
      onClose()
    }
  }

  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            Enregistrer
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={confirm}>
        {refusal}
        {about && <p>{about}</p>}
        {children}
        <Choices value={on} onChange={setOn} label={label} describedBy={describedBy}>
          {options.map((o) => (
            <Choice value={o.value} key={o.value} disabled={alone && on.includes(o.value)}>
              {o.label}
            </Choice>
          ))}
        </Choices>
        {last && <p>{last}</p>}
      </form>
    </Modal>
  )
}

function SessionsForm({ account, path, onClose }: { account: Account; path: AccountPath; onClose: () => void }) {
  const [sessions, setSessions] = useState(String(account.max_sessions))
  return (
    <Settings
      title={`Sessions du compte ${account.name}`}
      label="Types de bind"
      initial={account.allowed_bind_types}
      options={bindTypes.map((t) => ({ value: t, label: t }))}
      last="Au moins un type de bind."
      onClose={onClose}
      save={(on) => () =>
        gw.PATCH('/admin/accounts/{id}/session-limits', {
          ...path,
          body: { max_sessions: Number(sessions), allowed_bind_types: bindTypes.filter((t) => on.includes(t)) },
        })
      }
    >
      <Field
        label="Sessions simultanées"
        type="number"
        required
        min={0}
        max={2147483647}
        step={1}
        value={sessions}
        onChange={(e) => setSessions(e.target.value)}
      />
    </Settings>
  )
}
