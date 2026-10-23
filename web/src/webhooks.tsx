import { useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { type Account, accountsKey } from './accounts'
import type { components } from './admin.gen'
import { gw } from './gw'
import { Failure } from './list'
import { read } from './queries'
import { useShellMe } from './session'
import {
  Button,
  Confirmation,
  DataTable,
  Field,
  LoadingState,
  Modal,
  needs,
  SecretOnce,
  Select,
  Toolbar,
  useSubmission,
} from './ui'

type Webhook = components['schemas']['Webhook']
type Event = Webhook['event_type']
type Shown = components['schemas']['WebhookWithSecret']

const events: Record<Event, string> = { mo: 'Messages entrants (MO)', dlr: 'Accusés de réception (DLR)' }
const statuses: Record<Webhook['status'], string> = { active: 'Actif', disabled: 'Désactivé' }
const closed = 'Ce compte est fermé.'
const unsealed = 'Le scellement est indisponible : rien n’a été modifié.'
const https = 'https://[^\\/?#@]+([\\/?#].*)?'

export function Webhooks({ account }: { account: Account }) {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const heading = useId()
  const queryClient = useQueryClient()
  const key = [...accountsKey, account.id, 'webhooks']
  const webhooks = useQuery({
    queryKey: key,
    queryFn: () => read(gw.GET('/admin/accounts/{id}/webhooks', { params: { path: { id: account.id } } }), 'webhooks'),
  })
  const [pending, setPending] = useState<{ action: 'create' | 'edit' | 'rotate' | 'delete'; w?: Webhook }>()
  const [shown, setShown] = useState<Shown>()
  const close = () => setPending(undefined)
  const write = held.includes('accounts:write') ? undefined : needs('accounts:write')
  const frozen = write ?? (account.status === 'closed' ? 'Fermé définitivement.' : undefined)
  const list = webhooks.data ?? []
  const free = (Object.keys(events) as Event[]).filter((e) => !list.some((w) => w.event_type === e))
  const path = (w: Webhook) => ({ params: { path: { id: account.id, webhook_id: w.id } } })
  const reveal = (s: Shown) => {
    void queryClient.invalidateQueries({ queryKey: key })
    setPending(undefined)
    setShown(s)
  }

  return (
    <section aria-labelledby={heading}>
      <h2 id={heading}>Webhooks</h2>
      <Toolbar>
        <Button
          size="sm"
          blockedReason={frozen ?? (free.length === 0 ? 'Un webhook par événement au plus.' : undefined)}
          onClick={() => setPending({ action: 'create' })}
        >
          Nouveau webhook
        </Button>
      </Toolbar>
      {webhooks.isPending ? (
        <LoadingState label="Chargement des webhooks…" rows={2} />
      ) : webhooks.error ? (
        <Failure
          title="Les webhooks n’ont pas pu être chargés"
          error={webhooks.error}
          retry={() => void webhooks.refetch()}
        />
      ) : list.length === 0 ? (
        <p>Aucun webhook.</p>
      ) : (
        <DataTable<Webhook>
          caption="Webhooks du compte"
          rows={list}
          rowKey={(w) => w.id}
          columns={[
            { header: 'Événement', cell: (w) => events[w.event_type] },
            { header: 'URL', cell: (w) => w.url },
            { header: 'Statut', cell: (w) => statuses[w.status] },
            {
              header: 'Actions',
              cell: (w) => (
                <>
                  <Button size="sm" blockedReason={write} onClick={() => setPending({ action: 'edit', w })}>
                    Modifier
                  </Button>
                  <Button size="sm" blockedReason={frozen} onClick={() => setPending({ action: 'rotate', w })}>
                    Rotation du secret
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    blockedReason={write}
                    onClick={() => setPending({ action: 'delete', w })}
                  >
                    Supprimer
                  </Button>
                </>
              ),
            },
          ]}
        />
      )}
      {pending?.action === 'create' && (
        <CreateForm accountId={account.id} free={free} onClose={close} onCreated={reveal} />
      )}
      {pending?.action === 'edit' && pending.w && (
        <EditForm accountId={account.id} webhook={pending.w} onClose={close} invalidate={key} />
      )}
      {pending?.action === 'rotate' && pending.w && (
        <RotateForm accountId={account.id} webhook={pending.w} onClose={close} onRotated={reveal} />
      )}
      {pending?.action === 'delete' && pending.w && (
        <Confirmation
          title={`Supprimer le webhook ${events[pending.w.event_type]}`}
          action="Supprimer"
          danger
          invalidate={key}
          onClose={close}
          call={() => gw.DELETE('/admin/accounts/{id}/webhooks/{webhook_id}', path(pending.w as Webhook))}
        >
          <p>
            Les {pending.w.event_type === 'mo' ? 'messages entrants' : 'accusés de réception'} de ce compte ne sont plus
            remis par webhook.
          </p>
        </Confirmation>
      )}
      {shown && (
        <SecretOnce
          title={`Webhook ${events[shown.event_type]}`}
          secret={shown.secret}
          onClose={() => setShown(undefined)}
        >
          <p>Ce secret signe chaque appel en HMAC-SHA256. Il ne sera plus jamais affiché.</p>
        </SecretOnce>
      )}
    </section>
  )
}

function Form({
  title,
  action,
  waiting,
  onClose,
  onSubmit,
  children,
}: {
  title: string
  action: string
  waiting: boolean
  onClose: () => void
  onSubmit: (event: FormEvent) => void
  children: ReactNode
}) {
  const form = useId()
  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            {action}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={onSubmit}>
        {children}
      </form>
    </Modal>
  )
}

function UrlField({ value, onChange }: { value: string; onChange: (url: string) => void }) {
  return (
    <Field
      label="URL"
      type="url"
      required
      maxLength={2048}
      pattern={https}
      placeholder="https://hooks.example.com/sms"
      autoComplete="off"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

function CreateForm({
  accountId,
  free,
  onClose,
  onCreated,
}: {
  accountId: string
  free: Event[]
  onClose: () => void
  onCreated: (s: Shown) => void
}) {
  const { submit, waiting, refusal, fail } = useSubmission()
  const [event, setEvent] = useState<Event>(free[0] ?? 'mo')
  const [url, setUrl] = useState('')

  async function save(e: FormEvent) {
    e.preventDefault()
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/webhooks', {
        params: { path: { id: accountId } },
        body: { event_type: event, url },
      }),
    )
    const said = {
      409: 'Ce compte a déjà un webhook pour cet événement.',
      422: 'Ce compte est fermé ou n’existe plus.',
      503: unsealed,
    }[outcome.response?.status ?? 0]
    if (said) {
      fail(said)
      return
    }
    if ('data' in outcome) {
      onCreated(outcome.data)
    }
  }

  return (
    <Form title="Nouveau webhook" action="Créer" waiting={waiting} onClose={onClose} onSubmit={save}>
      {refusal}
      <Select
        label="Événement"
        items={free.map((e) => ({ value: e, label: events[e] }))}
        value={event}
        onChange={(next) => setEvent(next as Event)}
      />
      <UrlField value={url} onChange={setUrl} />
      <p>Le secret de signature est généré par la passerelle et montré une seule fois.</p>
    </Form>
  )
}

function EditForm({
  accountId,
  webhook,
  onClose,
  invalidate,
}: {
  accountId: string
  webhook: Webhook
  onClose: () => void
  invalidate: unknown[]
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail } = useSubmission()
  const [url, setUrl] = useState(webhook.url)
  const [status, setStatus] = useState<string>(webhook.status)

  async function save(e: FormEvent) {
    e.preventDefault()
    const outcome = await submit(() =>
      gw.PATCH('/admin/accounts/{id}/webhooks/{webhook_id}', {
        params: { path: { id: accountId, webhook_id: webhook.id } },
        body: {
          ...(url !== webhook.url && { url }),
          ...(status !== webhook.status && { status: status as Webhook['status'] }),
        },
      }),
    )
    if (outcome.response?.status === 409) {
      fail(closed)
      return
    }
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: invalidate })
      onClose()
    }
  }

  return (
    <Form
      title={`Modifier le webhook ${events[webhook.event_type]}`}
      action="Enregistrer"
      waiting={waiting}
      onClose={onClose}
      onSubmit={save}
    >
      {refusal}
      <UrlField value={url} onChange={setUrl} />
      <Select
        label="Statut"
        items={Object.entries(statuses).map(([value, label]) => ({ value, label }))}
        value={status}
        onChange={setStatus}
      />
    </Form>
  )
}

function RotateForm({
  accountId,
  webhook,
  onClose,
  onRotated,
}: {
  accountId: string
  webhook: Webhook
  onClose: () => void
  onRotated: (s: Shown) => void
}) {
  const { submit, waiting, refusal, fail } = useSubmission()

  async function save(e: FormEvent) {
    e.preventDefault()
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/webhooks/{webhook_id}/rotate-secret', {
        params: { path: { id: accountId, webhook_id: webhook.id } },
      }),
    )
    const said = { 409: closed, 503: unsealed }[outcome.response?.status ?? 0]
    if (said) {
      fail(said)
      return
    }
    if ('data' in outcome) {
      onRotated(outcome.data)
    }
  }

  return (
    <Form
      title={`Rotation du secret : ${events[webhook.event_type]}`}
      action="Tourner le secret"
      waiting={waiting}
      onClose={onClose}
      onSubmit={save}
    >
      {refusal}
      <p>L’ancien secret cesse immédiatement d’être accepté.</p>
    </Form>
  )
}
