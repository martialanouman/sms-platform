import { useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useId, useState } from 'react'
import { type Account, accountsKey } from './accounts'
import type { components } from './admin.gen'
import { at } from './audit'
import { gw } from './gw'
import { read } from './queries'
import { useShellMe } from './session'
import { Button, Confirmation, DataTable, Field, Modal, SecretOnce, Select, Toolbar, useSubmission } from './ui'

type Credential = components['schemas']['Credential']
type Kind = Credential['type']
type Shown = { credential: Credential; secret: string }

const revokedOnly = 'Révoqué : seule une rotation le ravive.'

const credentialStatus: Record<Credential['status'], string> = {
  active: 'Actif',
  disabled: 'Désactivé',
  revoked: 'Révoqué',
}

const kinds: Record<Kind, string> = { api_key: 'Clé d’API', smpp_bind: 'Bind SMPP' }

const named = (c: Credential) => (c.system_id ? `${kinds[c.type]} ${c.system_id}` : kinds[c.type])

const graces = [
  { value: '0', label: 'Aucune' },
  { value: '3600', label: '1 h' },
  { value: '86400', label: '24 h' },
  { value: '604800', label: '7 jours' },
]

function grace(c: Credential) {
  if (!c.grace_expires_at) {
    return '—'
  }
  const until = at(c.grace_expires_at)
  return new Date(c.grace_expires_at).getTime() > Date.now() ? `jusqu’au ${until}` : `expirée le ${until}`
}

export function Credentials({ account }: { account: Account }) {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const heading = useId()
  const key = [...accountsKey, account.id, 'credentials']
  const readable = held.includes('credentials:read')
  const credentials = useQuery({
    queryKey: key,
    queryFn: () =>
      read(gw.GET('/admin/accounts/{id}/credentials', { params: { path: { id: account.id } } }), 'credentials'),
    enabled: readable,
  })
  const [pending, setPending] = useState<{
    action: 'create' | 'rotate' | 'disable' | 'enable' | 'revoke'
    c?: Credential
  }>()
  const [shown, setShown] = useState<Shown>()
  const close = () => setPending(undefined)
  const blocked =
    (held.includes('credentials:write') ? undefined : 'Cette action demande credentials:write.') ??
    (account.status === 'closed' ? 'Clos définitivement.' : undefined)
  const list = credentials.data ?? []
  const free = (Object.keys(kinds) as Kind[]).filter((k) => !list.some((c) => c.type === k))
  const path = (c: Credential) => ({ params: { path: { id: account.id, credential_id: c.id } } })
  const reveal = (s: Shown) => {
    setPending(undefined)
    setShown(s)
  }

  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>Identifiants</h2>
      {!readable ? (
        <p>Les identifiants demandent credentials:read.</p>
      ) : (
        <>
          <Toolbar>
            <Button
              size="sm"
              blockedReason={blocked ?? (free.length === 0 ? 'Un identifiant de chaque type au plus.' : undefined)}
              onClick={() => setPending({ action: 'create' })}
            >
              Nouvel identifiant
            </Button>
          </Toolbar>
          {list.length === 0 ? (
            <p>{credentials.isPending ? 'Chargement des identifiants…' : 'Aucun identifiant.'}</p>
          ) : (
            <DataTable<Credential>
              caption="Identifiants du compte"
              rows={list}
              rowKey={(c) => c.id}
              columns={[
                { header: 'Identifiant', cell: named },
                { header: 'Statut', cell: (c) => credentialStatus[c.status] },
                { header: 'Grâce', cell: grace },
                { header: 'Dernier usage', cell: (c) => (c.last_used_at ? at(c.last_used_at) : '—') },
                { header: 'Rotation', cell: (c) => (c.rotated_at ? at(c.rotated_at) : '—') },
                {
                  header: 'Actions',
                  cell: (c) => {
                    const revoked = c.status === 'revoked' ? revokedOnly : undefined
                    const toggle = c.status === 'active' ? 'disable' : 'enable'
                    return (
                      <>
                        <Button size="sm" blockedReason={blocked} onClick={() => setPending({ action: 'rotate', c })}>
                          Rotation
                        </Button>
                        <Button
                          size="sm"
                          blockedReason={blocked ?? revoked}
                          onClick={() => setPending({ action: toggle, c })}
                        >
                          {toggle === 'enable' ? 'Activer' : 'Désactiver'}
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          blockedReason={blocked ?? (revoked && 'Déjà révoqué.')}
                          onClick={() => setPending({ action: 'revoke', c })}
                        >
                          Révoquer
                        </Button>
                      </>
                    )
                  },
                },
              ]}
            />
          )}
        </>
      )}
      {pending?.action === 'create' && (
        <CreateForm accountId={account.id} free={free} onClose={close} onCreated={reveal} />
      )}
      {pending?.action === 'rotate' && pending.c && (
        <RotateForm accountId={account.id} credential={pending.c} onClose={close} onRotated={reveal} />
      )}
      {(pending?.action === 'disable' || pending?.action === 'enable') && pending.c && (
        <Confirmation
          title={`${pending.action === 'disable' ? 'Désactiver' : 'Activer'} : ${named(pending.c)}`}
          action={pending.action === 'disable' ? 'Désactiver' : 'Activer'}
          invalidate={key}
          onClose={close}
          refusals={{ 409: revokedOnly }}
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
          call={() => gw.DELETE('/admin/accounts/{id}/credentials/{credential_id}', path(pending.c as Credential))}
        >
          <p>Le secret est effacé ; seule une rotation ravive l’identifiant.</p>
          {pending.c.system_id && <p>Le system_id {pending.c.system_id} reste réservé à ce compte.</p>}
        </Confirmation>
      )}
      {shown && (
        <SecretOnce title={named(shown.credential)} secret={shown.secret} onClose={() => setShown(undefined)}>
          <p>Ce secret ne sera plus jamais affiché.</p>
        </SecretOnce>
      )}
    </section>
  )
}

function CreateForm({
  accountId,
  free,
  onClose,
  onCreated,
}: {
  accountId: string
  free: Kind[]
  onClose: () => void
  onCreated: (s: Shown) => void
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const [kind, setKind] = useState<Kind>(free.includes('smpp_bind') ? 'smpp_bind' : 'api_key')
  const [systemId, setSystemId] = useState('')
  const form = useId()

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/credentials', {
        params: { path: { id: accountId } },
        body: kind === 'smpp_bind' ? { type: kind, system_id: systemId } : { type: kind },
      }),
    )
    const said = {
      409: kind === 'smpp_bind' ? 'Ce system_id est déjà pris.' : 'Ce compte a déjà un identifiant de ce type.',
      422: 'Ce compte est clos ou n’existe plus.',
    }[outcome.response?.status ?? 0]
    if (said) {
      fail(said)
      return
    }
    if ('data' in outcome) {
      const { secret, ...credential } = outcome.data
      void queryClient.invalidateQueries({ queryKey: [...accountsKey, accountId, 'credentials'] })
      onCreated({ credential, secret })
    }
  }

  return (
    <Modal
      title="Nouvel identifiant"
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
        <Select
          label="Type"
          items={free.map((k) => ({ value: k, label: kinds[k] }))}
          value={kind}
          onChange={(next) => setKind(next as Kind)}
        />
        {kind === 'smpp_bind' && (
          <Field
            label="system_id"
            required
            maxLength={15}
            pattern="[!-~]{1,15}"
            autoComplete="off"
            value={systemId}
            onChange={(e) => setSystemId(e.target.value)}
            aria-describedby={describedBy}
          />
        )}
        <p>Le secret est généré par la passerelle et montré une seule fois.</p>
      </form>
    </Modal>
  )
}

function RotateForm({
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
  const queryClient = useQueryClient()
  const { submit, waiting, refusal } = useSubmission()
  const [seconds, setSeconds] = useState('0')
  const form = useId()
  const revoked = credential.status === 'revoked'

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/credentials/{credential_id}/rotate', {
        params: { path: { id: accountId, credential_id: credential.id } },
        body: { grace_period_sec: Number(seconds) },
      }),
    )
    if ('data' in outcome) {
      const { secret, ...rotated } = outcome.data
      void queryClient.invalidateQueries({ queryKey: [...accountsKey, accountId, 'credentials'] })
      onRotated({ credential: rotated, secret })
    }
  }

  return (
    <Modal
      title={`Rotation ${credential.type === 'api_key' ? 'de la clé d’API' : `du bind SMPP ${credential.system_id}`}`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            Tourner le secret
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        {refusal}
        {revoked ? (
          <p>Révoqué : il redevient actif avec un secret neuf, sans grâce.</p>
        ) : (
          <>
            <Select label="Grâce" items={graces} value={seconds} onChange={setSeconds} />
            <p>Pendant la grâce, l’ancien secret reste accepté.</p>
            {credential.type === 'smpp_bind' && seconds === '0' && (
              <p>Sans grâce, les binds ouverts sont déconnectés.</p>
            )}
          </>
        )}
        {credential.status === 'disabled' && <p>L’identifiant reste désactivé.</p>}
      </form>
    </Modal>
  )
}
