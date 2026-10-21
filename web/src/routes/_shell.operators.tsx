import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { api, json, type send, unreachable } from '../api'
import type { components } from '../api.gen'
import { operatorsQuery, rolesQuery } from '../queries'
import { meQuery, Refused, Unauthenticated, useSessionClosed, useShellMe } from '../session'
import {
  Button,
  DataTable,
  EmptyState,
  ErrorState,
  Field,
  LoadingState,
  Modal,
  PageHeader,
  Toolbar,
  useSubmission,
} from '../ui'

type Operator = components['schemas']['Operator']

export const Route = createFileRoute('/_shell/operators')({ component: Operators })

function status(o: Operator) {
  if (o.status === 'disabled') {
    return 'Désactivé'
  }
  const link = o.accessLink
  if (!link) {
    return 'Actif'
  }
  if (link.state === 'failed') {
    return 'Envoi en échec'
  }
  if (link.state === 'sent') {
    return 'Lien envoyé'
  }
  return link.kind === 'activation' ? 'Activation en attente' : 'Envoi en attente'
}

type Pending = { kind: 'create' } | { kind: 'roles' | 'disable' | 'link'; operator: Operator }

function Operators() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('operators:read')
  const operators = useQuery({ ...operatorsQuery, enabled: readable })
  const closed = useSessionClosed()
  useEffect(() => {
    if (operators.error instanceof Unauthenticated) {
      closed()
    }
  }, [operators.error, closed])
  const writes = held.includes('operators:write')
  const writable = writes ? undefined : 'Cette action demande operators:write.'
  const readsRoles = held.includes('roles:read')
  const [pending, setPending] = useState<Pending>()
  const [note, setNote] = useState('')

  const title = useRef<HTMLHeadingElement>(null)
  const wasReadable = useRef(readable)
  useEffect(() => {
    if (wasReadable.current && !readable) {
      setPending(undefined)
      title.current?.focus()
    }
    wasReadable.current = readable
  }, [readable])

  const open = (p: Pending) => () => setPending(p)
  const dismiss = (p: Pending) => () => setPending((now) => (now === p ? undefined : now))

  return (
    <>
      <PageHeader title="Opérateurs" titleRef={title} />
      {readable ? (
        <>
          <Toolbar>
            <Button
              size="sm"
              variant="primary"
              blockedReason={writes ? undefined : 'La création d’un opérateur demande operators:write.'}
              onClick={open({ kind: 'create' })}
            >
              Nouvel opérateur
            </Button>
          </Toolbar>
          <section className="page">
            <p className="page__note" role="status">
              {note}
            </p>
            {operators.isPending ? (
              <LoadingState label="Chargement des opérateurs…" rows={3} />
            ) : operators.error ? (
              <ErrorState title="Les opérateurs n’ont pas pu être chargés" onRetry={() => void operators.refetch()}>
                {operators.error instanceof Refused ? operators.error.message : unreachable}
              </ErrorState>
            ) : operators.data.length === 0 ? (
              <EmptyState title="Aucun opérateur" />
            ) : (
              <DataTable
                caption="Opérateurs du tableau de bord"
                rows={operators.data}
                rowKey={(o) => o.id}
                columns={[
                  {
                    header: 'Opérateur',
                    cell: (o) => (
                      <span className="operator-cell">
                        <span>{o.displayName}</span>
                        <span className="operator-cell__email">{o.email}</span>
                      </span>
                    ),
                  },
                  { header: 'Statut', cell: status },
                  {
                    header: 'Rôles',
                    cell: (o) => (o.roles.length > 0 ? o.roles.map((r) => r.name).join(', ') : 'Aucun rôle'),
                  },
                  {
                    header: 'Second facteur',
                    cell: (o) => (o.secondFactorEnrolled ? 'Configuré' : 'Aucun'),
                  },
                  {
                    header: 'Actions',
                    cell: (o) => (
                      <div className="row-actions">
                        <Button
                          size="sm"
                          blockedReason={
                            writable ?? (readsRoles ? undefined : 'La liste des rôles à attribuer demande roles:read.')
                          }
                          onClick={open({ kind: 'roles', operator: o })}
                        >
                          Modifier les rôles
                        </Button>
                        <Status
                          operator={o}
                          blockedReason={
                            writable ??
                            (o.id === me.data?.operator.id
                              ? 'Le compte de la session ne se désactive pas ici : un autre détenteur de operators:write peut le faire.'
                              : undefined)
                          }
                          onDisable={open({ kind: 'disable', operator: o })}
                        />
                        <Button
                          size="sm"
                          blockedReason={
                            writable ??
                            (o.status === 'disabled'
                              ? 'Le compte est désactivé : réactivez-le avant d’envoyer un lien.'
                              : undefined)
                          }
                          onClick={open({ kind: 'link', operator: o })}
                        >
                          Envoyer un lien
                        </Button>
                      </div>
                    ),
                  },
                ]}
              />
            )}
            {pending?.kind === 'create' && (
              <Creation
                onClose={dismiss(pending)}
                onCreated={(email) => setNote(`Compte créé. Le lien d’activation part à ${email} ; il vaut 72 heures.`)}
              />
            )}
            {pending?.kind === 'roles' && (
              <Roles
                operator={pending.operator}
                self={pending.operator.id === me.data?.operator.id}
                onClose={dismiss(pending)}
              />
            )}
            {pending?.kind === 'disable' && <Disable operator={pending.operator} onClose={dismiss(pending)} />}
            {pending?.kind === 'link' && <AccessLink operator={pending.operator} onClose={dismiss(pending)} />}
          </section>
        </>
      ) : (
        <section className="page">
          <p className="auth__intro">Il faut la permission operators:read pour lire les opérateurs.</p>
        </section>
      )}
    </>
  )
}

function Creation({ onClose, onCreated }: { onClose: () => void; onCreated: (email: string) => void }) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const form = useId()

  async function create(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() => api.POST('/operators', { body: { email, displayName }, headers: json }))
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
      onCreated(outcome.data.email)
      onClose()
    }
  }

  return (
    <Modal
      title="Nouvel opérateur"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            Créer l’opérateur
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={create}>
        <p>
          Le compte naît actif, sans rôle ni mot de passe : son titulaire le définit par un lien d’activation reçu par
          e-mail, puis enrôle son second facteur. L’action est enregistrée dans le journal d’audit.
        </p>
        {refusal}
        <Field
          label="Adresse e-mail"
          type="email"
          required
          autoComplete="off"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          aria-describedby={describedBy}
        />
        <Field
          label="Nom affiché"
          required
          autoComplete="off"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
      </form>
    </Modal>
  )
}

function Roles({ operator, self, onClose }: { operator: Operator; self: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const roles = useQuery(rolesQuery)
  const [chosen, setChosen] = useState(operator.roles.map((r) => r.id))
  const { submit, waiting, refusal } = useSubmission()

  async function save() {
    const outcome = await submit(() =>
      api.PUT('/operators/{operatorId}/roles', {
        params: { path: { operatorId: operator.id } },
        body: { roleIds: chosen },
        headers: json,
      }),
    )
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
      if (self) {
        void queryClient.invalidateQueries({ queryKey: meQuery.queryKey })
      }
      onClose()
    }
  }

  return (
    <Modal
      title={`Rôles de ${operator.displayName}`}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button
            variant="primary"
            loading={waiting || roles.isPending}
            blockedReason={
              roles.error ? 'Les rôles n’ont pas pu être chargés : rien ne peut être enregistré.' : undefined
            }
            onClick={() => void save()}
          >
            Enregistrer les rôles
          </Button>
        </>
      }
    >
      {refusal}
      {roles.error && (
        <ErrorState
          title="Les rôles n’ont pas pu être chargés : rien ne peut être enregistré"
          onRetry={() => void roles.refetch()}
        >
          {roles.error instanceof Refused ? roles.error.message : unreachable}
        </ErrorState>
      )}
      {roles.isPending && <LoadingState label="Chargement des rôles…" rows={2} line />}
      {roles.data && (
        <fieldset className="role-choice">
          <legend>Rôles</legend>
          <p>
            Les permissions s’additionnent : l’opérateur détient l’union de ses rôles, dès sa prochaine requête.
            L’action est enregistrée dans le journal d’audit.
          </p>
          {roles.data.map((r) => (
            <label key={r.id} className="role-choice__option">
              <input
                type="checkbox"
                checked={chosen.includes(r.id)}
                onChange={(e) => setChosen(e.target.checked ? [...chosen, r.id] : chosen.filter((id) => id !== r.id))}
              />
              <span>{r.name}</span> <span className="role-choice__description">{r.description}</span>
            </label>
          ))}
        </fieldset>
      )}
    </Modal>
  )
}

function Confirmation({
  title,
  action,
  danger,
  call,
  onClose,
  children,
}: {
  title: string
  action: string
  danger?: boolean
  call: Parameters<typeof send>[0]
  onClose: () => void
  children: ReactNode
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal } = useSubmission()

  async function confirm() {
    const outcome = await submit(call)
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
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
          <Button variant={danger ? 'danger' : 'primary'} loading={waiting} onClick={() => void confirm()}>
            {action}
          </Button>
        </>
      }
    >
      {refusal}
      {children}
      <p>L’action est enregistrée dans le journal d’audit.</p>
    </Modal>
  )
}

function Disable({ operator, onClose }: { operator: Operator; onClose: () => void }) {
  return (
    <Confirmation
      title={`Désactiver ${operator.displayName} ?`}
      action="Désactiver"
      danger
      onClose={onClose}
      call={() =>
        api.PATCH('/operators/{operatorId}', {
          params: { path: { operatorId: operator.id } },
          body: { status: 'disabled' },
          headers: json,
        })
      }
    >
      <p>
        Ses sessions seront fermées immédiatement et il ne pourra plus se connecter. Ses rôles, son second facteur et un
        lien d’accès en cours resteront en place : « Réactiver » les lui rendra.
      </p>
    </Confirmation>
  )
}

function AccessLink({ operator, onClose }: { operator: Operator; onClose: () => void }) {
  const kind = operator.accessLink?.kind ?? 'unknown'
  const title = {
    activation: `Envoyer un lien d’activation à ${operator.displayName} ?`,
    reset: `Envoyer un lien de réinitialisation à ${operator.displayName} ?`,
    unknown: `Envoyer un lien à ${operator.displayName} ?`,
  }[kind]
  const effect = {
    activation: `Un lien d’activation sera envoyé à ${operator.email} ; il restera valable 72 heures.`,
    reset: `Un lien de réinitialisation sera envoyé à ${operator.email} ; il restera valable 1 heure. Son mot de passe, son second facteur et ses sessions resteront valables jusqu’à son usage. À l’usage, l’opérateur définira un nouveau mot de passe, son second facteur sera retiré et ses sessions seront fermées.`,
    unknown: `${operator.email} recevra un lien à usage unique : activation (72 h) si le compte n’a pas encore de mot de passe, réinitialisation (1 h) sinon.`,
  }[kind]

  return (
    <Confirmation
      title={title}
      action="Envoyer"
      onClose={onClose}
      call={() =>
        api.POST('/operators/{operatorId}/access-link', {
          params: { path: { operatorId: operator.id } },
          headers: json,
        })
      }
    >
      <p>{effect} Le lien envoyé plus tôt cesse de valoir dès cette demande.</p>
    </Confirmation>
  )
}

function Status({
  operator,
  blockedReason,
  onDisable,
}: {
  operator: Operator
  blockedReason?: string
  onDisable: () => void
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal } = useSubmission()
  const active = operator.status === 'active'

  async function enable() {
    const outcome = await submit(() =>
      api.PATCH('/operators/{operatorId}', {
        params: { path: { operatorId: operator.id } },
        body: { status: 'active' },
        headers: json,
      }),
    )
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
    }
  }

  return (
    <>
      <Button
        size="sm"
        variant={active ? 'danger' : 'secondary'}
        blockedReason={blockedReason}
        loading={waiting}
        onClick={active ? onDisable : () => void enable()}
      >
        {active ? 'Désactiver' : 'Réactiver'}
      </Button>
      {refusal}
    </>
  )
}
