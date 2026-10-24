import { useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { accountsKey, useAccount } from './accounts'
import type { components } from './admin.gen'
import { atMinute } from './audit'
import { gw } from './gw'
import { Failure } from './list'
import { read } from './queries'
import { useShellMe } from './session'
import {
  Button,
  Card,
  Confirmation,
  Field,
  KeyValue,
  LoadingState,
  Modal,
  needs,
  RotationModal,
  SecretOnce,
  Select,
  StatusPill,
  type Tone,
  useSubmission,
  useToast,
} from './ui'

type Credential = components['schemas']['Credential']
type Kind = Credential['type']
type Shown = components['schemas']['CredentialWithSecret']
type Action = 'create' | 'rotate' | 'disable' | 'enable' | 'revoke'

const statuses: Record<Credential['status'], [Tone, string]> = {
  active: ['ok', 'Actif'],
  disabled: ['neutral', 'Désactivé'],
  revoked: ['bad', 'Révoqué'],
}

const kinds: Record<Kind, string> = { smpp_bind: 'Bind SMPP', api_key: 'Clé d’API' }

const empty: Record<Kind, { title: string; about: string; create: string }> = {
  smpp_bind: {
    title: 'Aucun bind SMPP',
    about: 'Le bind sert au canal SMPP. La passerelle génère le mot de passe et le montre une seule fois.',
    create: 'Créer un bind SMPP',
  },
  api_key: {
    title: 'Aucune clé d’API',
    about: 'La clé sert au canal REST. La passerelle la génère et la montre une seule fois.',
    create: 'Créer une clé d’API',
  },
}

const named = (c: Credential) => (c.system_id ? `${kinds[c.type]} ${c.system_id}` : kinds[c.type])

const graces = [
  { value: '0', label: 'Aucune' },
  { value: '3600', label: '1 h' },
  { value: '86400', label: '24 h' },
  { value: '604800', label: '7 jours' },
]

const when = (iso: string | null) =>
  iso ? <span className="ui-mono">{atMinute(iso)}</span> : <span className="ui-muted">jamais</span>

const mono = (text: string) => <span className="ui-mono">{text}</span>

export function systemIdFrom(name: string, random: () => number) {
  const root =
    name
      .normalize('NFD')
      .toLowerCase()
      .replace(/[^a-z0-9]/g, '')
      .slice(0, 10) || 'bind'
  return `${root}_${String(Math.floor(random() * 10000)).padStart(4, '0')}`
}

export function Credentials() {
  const { account } = useAccount()
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const key = [...accountsKey, account.id, 'credentials']
  const readable = held.includes('credentials:read')
  const credentials = useQuery({
    queryKey: key,
    queryFn: () =>
      read(gw.GET('/admin/accounts/{id}/credentials', { params: { path: { id: account.id } } }), 'credentials'),
    enabled: readable,
  })
  const queryClient = useQueryClient()
  const notify = useToast()
  const [pending, setPending] = useState<{ action: Action; c?: Credential; opener: Element | null }>()
  const [shown, setShown] = useState<{ secret: Shown; opener: Element | null }>()
  const close = () => setPending(undefined)
  const ask = (action: Action, c?: Credential) => setPending({ action, c, opener: document.activeElement })
  const creation = useSubmission()
  const [creating, setCreating] = useState(false)
  const blocked =
    (held.includes('credentials:write') ? undefined : needs('credentials:write')) ??
    (account.status === 'closed' ? 'Fermé définitivement.' : undefined) ??
    (creating ? 'Une clé d’API est en cours de création.' : undefined)
  const path = (c: Credential) => ({ params: { path: { id: account.id, credential_id: c.id } } })
  const reveal = (secret: Shown) => {
    void queryClient.invalidateQueries({ queryKey: key })
    close()
    setShown({ secret, opener: pending?.opener ?? null })
  }
  async function createKey() {
    setCreating(true)
    const outcome = await creation.submit(() =>
      gw.POST('/admin/accounts/{id}/credentials', { params: { path: { id: account.id } }, body: { type: 'api_key' } }),
    )
    setCreating(false)
    const status = outcome.response?.status
    if (status === 409) {
      document.querySelector<HTMLElement>('#contenu h1')?.focus()
      void queryClient.invalidateQueries({ queryKey: key })
    }
    if (status === 422) {
      creation.fail('Ce compte est fermé ou n’existe plus.')
    }
    if ('data' in outcome) {
      reveal(outcome.data)
    }
  }
  const done = (c: Credential, verb: string) => () => {
    notify(`${kinds[c.type]} ${verb}${c.type === 'api_key' ? 'e' : ''}.`)
    close()
  }

  if (!readable) {
    return <p>{needs('credentials:read', 'Cet onglet')}</p>
  }
  if (credentials.isPending) {
    return <LoadingState label="Chargement des identifiants…" rows={2} />
  }
  if (credentials.error) {
    return (
      <Failure
        title="Les identifiants n’ont pas pu être chargés"
        error={credentials.error}
        retry={() => void credentials.refetch()}
      />
    )
  }
  const list = credentials.data

  return (
    <>
      <div className="ui-cards">
        {(Object.keys(kinds) as Kind[]).map((kind) => {
          const c = list.find((one) => one.type === kind)
          return c ? (
            <Filled key={kind} credential={c} blocked={blocked} ask={(action) => ask(action, c)} />
          ) : (
            <Empty
              key={kind}
              kind={kind}
              blocked={blocked}
              waiting={kind === 'api_key' && creation.waiting}
              refusal={kind === 'api_key' && creation.refusal}
              onCreate={kind === 'smpp_bind' ? () => ask('create') : () => void createKey()}
            />
          )
        })}
      </div>
      {pending?.action === 'create' && (
        <CreateBind accountId={account.id} accountName={account.name} onClose={close} onCreated={reveal} />
      )}
      {pending?.action === 'rotate' && pending.c && (
        <Rotate accountId={account.id} credential={pending.c} onClose={close} onRotated={reveal} />
      )}
      {(pending?.action === 'disable' || pending?.action === 'enable') && pending.c && (
        <Confirmation
          title={`${pending.action === 'disable' ? 'Désactiver' : 'Activer'} : ${named(pending.c)}`}
          action={pending.action === 'disable' ? 'Désactiver' : 'Activer'}
          invalidate={key}
          onClose={close}
          onDone={done(pending.c, pending.action === 'disable' ? 'désactivé' : 'activé')}
          refusals={{ 409: 'Refusé : l’identifiant est révoqué, ou le compte est fermé.' }}
          call={() =>
            gw.PATCH('/admin/accounts/{id}/credentials/{credential_id}', {
              ...path(pending.c as Credential),
              body: { status: pending.action === 'disable' ? 'disabled' : 'active' },
            })
          }
        >
          {pending.action === 'disable' && pending.c.type === 'smpp_bind' && <p>Ses binds ouverts sont déconnectés.</p>}
        </Confirmation>
      )}
      {pending?.action === 'revoke' && pending.c && (
        <Confirmation
          title={`Révoquer : ${named(pending.c)}`}
          action="Révoquer"
          danger
          invalidate={key}
          onClose={close}
          onDone={done(pending.c, 'révoqué')}
          call={() => gw.DELETE('/admin/accounts/{id}/credentials/{credential_id}', path(pending.c as Credential))}
        >
          <p>Le secret est effacé ; seule une rotation ravive l’identifiant.</p>
          {pending.c.system_id && <p>Le System ID {pending.c.system_id} reste réservé à ce compte.</p>}
        </Confirmation>
      )}
      {shown && (
        <SecretOnce
          title={shown.secret.type === 'smpp_bind' ? 'Nouveau mot de passe' : 'Nouvelle clé d’API'}
          secret={shown.secret.secret}
          opener={shown.opener}
          onClose={() => setShown(undefined)}
        >
          <KeyValue
            items={[
              { label: 'Compte', value: mono(account.name) },
              ...(shown.secret.system_id ? [{ label: 'System ID', value: mono(shown.secret.system_id) }] : []),
            ]}
          />
        </SecretOnce>
      )}
    </>
  )
}

function Filled({
  credential: c,
  blocked,
  ask,
}: {
  credential: Credential
  blocked: ReactNode
  ask: (action: Action) => void
}) {
  const [tone, status] = statuses[c.status]
  const revoked = c.status === 'revoked'
  const toggle = c.status === 'active' ? 'disable' : 'enable'
  const grace = c.grace_expires_at && new Date(c.grace_expires_at).getTime() > Date.now() ? c.grace_expires_at : null
  return (
    <Card
      title={kinds[c.type]}
      aside={<StatusPill tone={tone}>{status}</StatusPill>}
      footer={
        <div className="ui-card__foot">
          <Button size="sm" blockedReason={blocked} onClick={() => ask('rotate')}>
            Rotation
          </Button>
          <Button
            size="sm"
            blockedReason={blocked ?? (revoked ? 'Révoqué : seule une rotation le ravive.' : undefined)}
            onClick={() => ask(toggle)}
          >
            {toggle === 'enable' ? 'Activer' : 'Désactiver'}
          </Button>
          <Button
            size="sm"
            variant="danger"
            blockedReason={blocked ?? (revoked ? 'Déjà révoqué.' : undefined)}
            onClick={() => ask('revoke')}
          >
            Révoquer
          </Button>
        </div>
      }
    >
      <KeyValue
        items={[
          ...(c.system_id ? [{ label: 'System ID', value: mono(c.system_id) }] : []),
          { label: 'Dernière utilisation', value: when(c.last_used_at) },
          { label: 'Dernière rotation', value: when(c.rotated_at) },
          ...(grace ? [{ label: 'Ancien secret', value: <>accepté jusqu’au {mono(atMinute(grace))}</> }] : []),
          { label: 'Créé le', value: <span className="ui-mono ui-muted">{atMinute(c.created_at)}</span> },
        ]}
      />
    </Card>
  )
}

function Empty({
  kind,
  blocked,
  waiting,
  refusal,
  onCreate,
}: {
  kind: Kind
  blocked: ReactNode
  waiting: boolean
  refusal: ReactNode
  onCreate: () => void
}) {
  const { title, about, create } = empty[kind]
  return (
    <Card title={kinds[kind]}>
      <div className="ui-card__empty">
        <p className="ui-card__empty-title">{title}</p>
        <p className="ui-muted">{about}</p>
        {refusal}
        <Button variant="primary" size="sm" loading={waiting} blockedReason={blocked} onClick={onCreate}>
          {create}
        </Button>
      </div>
    </Card>
  )
}

function CreateBind({
  accountId,
  accountName,
  onClose,
  onCreated,
}: {
  accountId: string
  accountName: string
  onClose: () => void
  onCreated: (s: Shown) => void
}) {
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const [systemId, setSystemId] = useState('')
  const form = useId()

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/credentials', {
        params: { path: { id: accountId } },
        body: { type: 'smpp_bind', system_id: systemId },
      }),
    )
    const said = { 409: 'Ce System ID est déjà pris.', 422: 'Ce compte est fermé ou n’existe plus.' }[
      outcome.response?.status ?? 0
    ]
    if (said) {
      fail(said)
    }
    if ('data' in outcome) {
      onCreated(outcome.data)
    }
  }

  return (
    <Modal
      title="Nouveau bind SMPP"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            Créer
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        {refusal}
        <div className="form__inline">
          <Field
            label="System ID"
            required
            maxLength={15}
            pattern="[!-~]{1,15}"
            autoComplete="off"
            value={systemId}
            onChange={(e) => setSystemId(e.target.value)}
            aria-describedby={describedBy}
          />
          <Button onClick={() => setSystemId(systemIdFrom(accountName, Math.random))}>Générer</Button>
        </div>
        <p>Le secret est généré par la passerelle et montré une seule fois.</p>
      </form>
    </Modal>
  )
}

function Rotate({
  accountId,
  credential,
  onClose,
  onRotated,
}: {
  accountId: string
  credential: Credential
  onClose: () => void
  onRotated: (s: Shown) => void
}) {
  const { submit, waiting, refusal, fail } = useSubmission()
  const [seconds, setSeconds] = useState('0')
  const revoked = credential.status === 'revoked'
  const bind = credential.type === 'smpp_bind'
  const grace = graces.find((g) => g.value === seconds)?.label

  async function rotate() {
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/credentials/{credential_id}/rotate', {
        params: { path: { id: accountId, credential_id: credential.id } },
        body: revoked ? {} : { grace_period_sec: Number(seconds) },
      }),
    )
    if (outcome.response?.status === 409) {
      fail('Ce compte est fermé.')
    }
    if ('data' in outcome) {
      onRotated(outcome.data)
    }
  }

  const warning = revoked
    ? 'Révoqué : il redevient actif avec un secret neuf, sans grâce.'
    : seconds === '0'
      ? bind
        ? 'Sans grâce, l’ancien mot de passe cesse aussitôt d’être accepté et les binds ouverts du compte sont déconnectés.'
        : 'Sans grâce, l’ancienne clé cesse aussitôt d’être acceptée.'
      : bind
        ? `Pendant ${grace}, l’ancien mot de passe reste accepté ; aucun bind n’est déconnecté.`
        : `Pendant ${grace}, l’ancienne clé reste acceptée.`

  return (
    <RotationModal
      title={bind ? <>Rotation du bind SMPP {mono(credential.system_id ?? '')}</> : 'Rotation de la clé d’API'}
      warning={
        <>
          <p>{warning}</p>
          {credential.status === 'disabled' && <p>L’identifiant reste désactivé.</p>}
        </>
      }
      onClose={onClose}
      onSubmit={() => void rotate()}
      waiting={waiting}
      refusal={refusal}
    >
      {!revoked && <Select label="Fenêtre de grâce" items={graces} value={seconds} onChange={setSeconds} />}
    </RotationModal>
  )
}
