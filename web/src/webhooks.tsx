import { useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, type ReactNode, useId, useState } from 'react'
import { accountsKey, useAccount } from './accounts'
import type { components } from './admin.gen'
import { gw } from './gw'
import { Failure } from './list'
import { read } from './queries'
import { useShellMe } from './session'
import {
  Button,
  Card,
  Confirmation,
  DataTable,
  EmptyState,
  Field,
  KeyValue,
  LoadingState,
  Modal,
  needs,
  RotationModal,
  SecretOnce,
  Select,
  StatusPill,
  useSubmission,
  useToast,
} from './ui'

type Webhook = components['schemas']['Webhook']
type Event = Webhook['event_type']
type Shown = components['schemas']['WebhookWithSecret']
type Action = 'create' | 'disable' | 'enable' | 'edit' | 'rotate' | 'delete'

const events: Record<Event, { short: string; about: string; plural: string }> = {
  mo: { short: 'MO', about: 'SMS entrants', plural: 'Les SMS entrants' },
  dlr: { short: 'DLR', about: 'accusés de réception', plural: 'Les accusés de réception' },
}
const closed = 'Ce compte est fermé.'
const unsealed = 'Le scellement est indisponible : rien n’a été modifié.'
const https = 'https://[^\\/?#@]+([\\/?#].*)?'

export function Webhooks() {
  const { account, frozen } = useAccount()
  const me = useShellMe()
  const queryClient = useQueryClient()
  const notify = useToast()
  const key = [...accountsKey, account.id, 'webhooks']
  const webhooks = useQuery({
    queryKey: key,
    queryFn: () => read(gw.GET('/admin/accounts/{id}/webhooks', { params: { path: { id: account.id } } }), 'webhooks'),
  })
  const [pending, setPending] = useState<{ action: Action; w?: Webhook; opener: Element | null }>()
  const [shown, setShown] = useState<{ secret: Shown; opener: Element | null }>()
  const close = () => setPending(undefined)
  const ask = (action: Action, w?: Webhook) => setPending({ action, w, opener: document.activeElement })
  const write = me.data?.permissions.includes('accounts:write') ? undefined : needs('accounts:write')
  const list = webhooks.data ?? []
  const free = (Object.keys(events) as Event[]).filter((e) => !list.some((w) => w.event_type === e))
  const path = (w: Webhook) => ({ params: { path: { id: account.id, webhook_id: w.id } } })
  const reveal = (secret: Shown) => {
    void queryClient.invalidateQueries({ queryKey: key })
    close()
    setShown({ secret, opener: pending?.opener ?? null })
  }
  const done = (text: string) => () => {
    notify(text)
    close()
  }

  return (
    <>
      <Card
        flush
        title="Webhooks MO / DLR"
        aside={
          webhooks.data && (
            <Button
              variant="primary"
              size="sm"
              blockedReason={frozen ?? (free.length === 0 ? 'Un webhook par événement au plus.' : undefined)}
              onClick={() => ask('create')}
            >
              Ajouter
            </Button>
          )
        }
      >
        {webhooks.isPending ? (
          <LoadingState label="Chargement des webhooks…" rows={2} />
        ) : webhooks.error ? (
          <Failure
            title="Les webhooks n’ont pas pu être chargés"
            error={webhooks.error}
            retry={() => void webhooks.refetch()}
          />
        ) : list.length === 0 ? (
          <EmptyState title="Aucun webhook">
            Les SMS entrants et les accusés de réception de ce compte ne sont remis à aucune URL.
          </EmptyState>
        ) : (
          <DataTable<Webhook>
            caption="Webhooks du compte"
            rows={list}
            rowKey={(w) => w.id}
            columns={[
              {
                header: 'Événement',
                cell: (w) => (
                  <>
                    <span className={`ui-badge ui-badge--${w.event_type}`}>{events[w.event_type].short}</span>
                    <span className="ui-why">{events[w.event_type].about}</span>
                  </>
                ),
              },
              { header: 'URL', cell: (w) => <span className="ui-mono">{w.url}</span> },
              {
                header: 'Statut',
                cell: (w) =>
                  w.status === 'active' ? (
                    <StatusPill tone="ok">Actif</StatusPill>
                  ) : (
                    <StatusPill tone="neutral">Désactivé</StatusPill>
                  ),
              },
              {
                header: 'Actions',
                end: true,
                cell: (w) => (
                  <div className="row-actions">
                    {w.status === 'active' ? (
                      <Button size="sm" blockedReason={write} onClick={() => ask('disable', w)}>
                        Désactiver
                      </Button>
                    ) : (
                      <Button size="sm" blockedReason={frozen} onClick={() => ask('enable', w)}>
                        Activer
                      </Button>
                    )}
                    <Button size="sm" blockedReason={write} onClick={() => ask('edit', w)}>
                      Modifier l’URL
                    </Button>
                    <Button size="sm" blockedReason={frozen} onClick={() => ask('rotate', w)}>
                      Rotation du secret
                    </Button>
                    <Button size="sm" variant="danger" blockedReason={write} onClick={() => ask('delete', w)}>
                      Supprimer
                    </Button>
                  </div>
                ),
              },
            ]}
          />
        )}
      </Card>
      <p className="ui-why">
        Le secret signe chaque appel en HMAC-SHA256 ; il n’est montré qu’à sa création ou à sa rotation, et la rotation
        coupe l’ancien aussitôt. Chaque action est enregistrée dans le journal d’audit.
      </p>
      {pending?.action === 'create' && (
        <CreateForm accountId={account.id} free={free} onClose={close} onCreated={reveal} />
      )}
      {(pending?.action === 'disable' || pending?.action === 'enable') && pending.w && (
        <Confirmation
          title={`${pending.action === 'disable' ? 'Désactiver' : 'Activer'} le webhook ${events[pending.w.event_type].short}`}
          action={pending.action === 'disable' ? 'Désactiver' : 'Activer'}
          invalidate={key}
          onClose={close}
          onDone={done(
            `Webhook ${events[pending.w.event_type].short} ${pending.action === 'disable' ? 'désactivé' : 'activé'}.`,
          )}
          refusals={{ 409: closed }}
          call={() =>
            gw.PATCH('/admin/accounts/{id}/webhooks/{webhook_id}', {
              ...path(pending.w as Webhook),
              body: { status: pending.action === 'disable' ? 'disabled' : 'active' },
            })
          }
        >
          <p>
            {events[pending.w.event_type].plural} de ce compte{' '}
            {pending.action === 'disable' ? 'ne sont plus remis' : 'sont de nouveau remis'} par webhook.
          </p>
        </Confirmation>
      )}
      {pending?.action === 'edit' && pending.w && (
        <EditForm
          accountId={account.id}
          webhook={pending.w}
          onClose={close}
          onSaved={done(`URL du webhook ${events[pending.w.event_type].short} modifiée.`)}
          invalidate={key}
        />
      )}
      {pending?.action === 'rotate' && pending.w && (
        <RotateForm accountId={account.id} webhook={pending.w} onClose={close} onRotated={reveal} />
      )}
      {pending?.action === 'delete' && pending.w && (
        <Confirmation
          title={`Supprimer le webhook ${events[pending.w.event_type].short}`}
          action="Supprimer"
          danger
          invalidate={key}
          onClose={close}
          onDone={() => {
            const gone = pending.w as Webhook
            queryClient.setQueryData<Webhook[]>(key, (rows) => rows?.filter((w) => w.id !== gone.id))
            done(`Webhook ${events[gone.event_type].short} supprimé.`)()
          }}
          call={() => gw.DELETE('/admin/accounts/{id}/webhooks/{webhook_id}', path(pending.w as Webhook))}
        >
          <p>{events[pending.w.event_type].plural} de ce compte ne sont plus remis par webhook.</p>
        </Confirmation>
      )}
      {shown && (
        <SecretOnce
          title={`Nouveau secret du webhook ${events[shown.secret.event_type].short}`}
          secret={shown.secret.secret}
          opener={shown.opener}
          onClose={() => setShown(undefined)}
        >
          <KeyValue
            items={[
              { label: 'Compte', value: <span className="ui-mono">{account.name}</span> },
              { label: 'Événement', value: events[shown.secret.event_type].short },
            ]}
          />
          <p>Ce secret signe chaque appel en HMAC-SHA256.</p>
        </SecretOnce>
      )}
    </>
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
        items={free.map((e) => ({ value: e, label: `${events[e].short} (${events[e].about})` }))}
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
  onSaved,
  invalidate,
}: {
  accountId: string
  webhook: Webhook
  onClose: () => void
  onSaved: () => void
  invalidate: unknown[]
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal } = useSubmission()
  const [url, setUrl] = useState(webhook.url)

  async function save(e: FormEvent) {
    e.preventDefault()
    const outcome = await submit(() =>
      gw.PATCH('/admin/accounts/{id}/webhooks/{webhook_id}', {
        params: { path: { id: accountId, webhook_id: webhook.id } },
        body: { url },
      }),
    )
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: invalidate })
      onSaved()
    }
  }

  return (
    <Form
      title={`Modifier l’URL du webhook ${events[webhook.event_type].short}`}
      action="Enregistrer"
      waiting={waiting}
      onClose={onClose}
      onSubmit={save}
    >
      {refusal}
      <UrlField value={url} onChange={setUrl} />
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

  async function rotate() {
    const outcome = await submit(() =>
      gw.POST('/admin/accounts/{id}/webhooks/{webhook_id}/rotate-secret', {
        params: { path: { id: accountId, webhook_id: webhook.id } },
      }),
    )
    const said = { 409: closed, 503: unsealed }[outcome.response?.status ?? 0]
    if (said) {
      fail(said)
    }
    if ('data' in outcome) {
      onRotated(outcome.data)
    }
  }

  return (
    <RotationModal
      title={`Rotation du secret du webhook ${events[webhook.event_type].short}`}
      warning="L’ancien secret cesse immédiatement d’être accepté."
      onClose={onClose}
      onSubmit={() => void rotate()}
      waiting={waiting}
      refusal={refusal}
    />
  )
}
