import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { api, json, unreachable } from '../api'
import type { components } from '../api.gen'
import { Failure, NoResults, text, useClosedOn } from '../list'
import { operatorsQuery, rolesQuery } from '../queries'
import { meQuery, Refused, useShellMe } from '../session'
import {
  Button,
  Card,
  Choice,
  Choices,
  Confirmation,
  DataTable,
  EmptyState,
  ErrorState,
  Field,
  Forbidden,
  LoadingState,
  Modal,
  needs,
  OperatorCell,
  PageHeader,
  Select,
  StatusPill,
  useSubmission,
  useToast,
} from '../ui'

type Operator = components['schemas']['Operator']
type OperatorStatus = Operator['status']
type OperatorsSearch = { status?: OperatorStatus; role?: string }

export const Route = createFileRoute('/_shell/operators')({
  validateSearch: (s: Record<string, unknown>): OperatorsSearch => ({
    status: s.status === 'active' || s.status === 'disabled' ? s.status : undefined,
    role: text(s.role),
  }),
  component: Operators,
})

const statuses = [
  { value: '', label: 'Tous' },
  { value: 'active', label: 'Actifs' },
  { value: 'disabled', label: 'Désactivés' },
]

function Status({ operator }: { operator: Operator }) {
  if (operator.status === 'disabled') {
    return <StatusPill tone="neutral">Désactivé</StatusPill>
  }
  if (operator.accessLink?.state === 'failed') {
    return <StatusPill tone="warn">Envoi en échec</StatusPill>
  }
  return <StatusPill tone="ok">Actif</StatusPill>
}

const link = (o: Operator) =>
  o.activated
    ? {
        action: 'Réinitialiser l’accès',
        sent: `Lien de réinitialisation envoyé à ${o.email}.`,
        title: `Envoyer un lien de réinitialisation à ${o.displayName} ?`,
        effect: `Un lien de réinitialisation sera envoyé à ${o.email} ; il restera valable 1 heure. Son mot de passe, son second facteur et ses sessions resteront valables jusqu’à son usage. À l’usage, l’opérateur définira un nouveau mot de passe, son second facteur sera retiré et ses sessions seront fermées.`,
      }
    : {
        action: 'Renvoyer l’activation',
        sent: `Lien d’activation renvoyé à ${o.email}.`,
        title: `Envoyer un lien d’activation à ${o.displayName} ?`,
        effect: `Un lien d’activation sera envoyé à ${o.email} ; il restera valable 72 heures.`,
      }

const shown = (n: number, total: number) => `${n} ${n === 1 ? 'entrée affichée' : 'entrées affichées'} sur ${total}`

type Pending = { kind: 'create' } | { kind: 'roles' | 'disable' | 'link'; operator: Operator }

function Operators() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('operators:read')
  const operators = useQuery({ ...operatorsQuery, enabled: readable })
  useClosedOn(operators.error)
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const notify = useToast()
  const writes = held.includes('operators:write')
  const writable = writes ? undefined : needs('operators:write')
  const readsRoles = held.includes('roles:read')
  const [pending, setPending] = useState<Pending>()

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
  const filter = (next: OperatorsSearch) => void navigate({ search: { ...search, ...next } })
  const clear = () => void navigate({ search: {} }).then(() => title.current?.focus())
  const all = operators.data ?? []
  const rows = all.filter(
    (o) =>
      (search.status === undefined || o.status === search.status) &&
      (search.role === undefined || o.roles.some((r) => r.id === search.role)),
  )
  const names = new Map(all.flatMap((o) => o.roles).map((r) => [r.id, r.name]))
  if (search.role !== undefined && !names.has(search.role)) {
    names.set(search.role, 'Rôle sans détenteur')
  }
  const roleFilters = [
    { value: '', label: 'Tous' },
    ...[...names.entries()]
      .sort(([, a], [, b]) => a.localeCompare(b, 'fr'))
      .map(([value, label]) => ({ value, label })),
  ]

  return (
    <>
      <PageHeader
        crumbs={['Opérateurs']}
        title="Opérateurs"
        titleRef={title}
        meta={
          readable &&
          'Les personnes qui se connectent au tableau de bord. Un opérateur se désactive, il ne se supprime pas : le journal tient ses auteurs.'
        }
        actions={
          readable && (
            <Button
              size="sm"
              variant="primary"
              blockedReason={writes ? undefined : needs('operators:write', 'La création d’un opérateur')}
              onClick={open({ kind: 'create' })}
            >
              Nouvel opérateur
            </Button>
          )
        }
      />
      {readable ? (
        <section className="page">
          <div className="ui-filters">
            <Select
              label="Statut"
              items={statuses}
              value={search.status ?? ''}
              onChange={(status) => filter({ status: status === '' ? undefined : (status as OperatorStatus) })}
            />
            <Select
              label="Rôle"
              items={roleFilters}
              value={search.role ?? ''}
              onChange={(role) => filter({ role: role === '' ? undefined : role })}
            />
          </div>
          {operators.isPending ? (
            <LoadingState label="Chargement des opérateurs…" rows={3} />
          ) : operators.error ? (
            <Failure
              title="Les opérateurs n’ont pas pu être chargés"
              error={operators.error}
              retry={() => void operators.refetch()}
            />
          ) : rows.length === 0 && (search.status !== undefined || search.role !== undefined) ? (
            <NoResults clear={clear} />
          ) : rows.length === 0 ? (
            <EmptyState title="Aucun opérateur" />
          ) : (
            <Card
              flush
              footer={
                <div className="ui-pager">
                  <span>{shown(rows.length, all.length)}</span>
                </div>
              }
            >
              <DataTable
                caption="Opérateurs du tableau de bord"
                rows={rows}
                rowKey={(o) => o.id}
                columns={[
                  {
                    header: 'Opérateur',
                    cell: (o) => <OperatorCell name={o.displayName} email={o.email} />,
                  },
                  { header: 'Statut', cell: (o) => <Status operator={o} /> },
                  {
                    header: 'Rôles',
                    cell: (o) =>
                      o.roles.length > 0 ? (
                        <span className="ui-tags">
                          {o.roles.map((r) => (
                            <span className="ui-tag" key={r.id}>
                              {r.name}
                            </span>
                          ))}
                        </span>
                      ) : (
                        <span className="ui-muted">Aucun rôle</span>
                      ),
                  },
                  {
                    header: 'Second facteur',
                    cell: (o) =>
                      o.secondFactorEnrolled ? (
                        <StatusPill tone="ok">Enrôlé</StatusPill>
                      ) : (
                        <StatusPill tone="neutral">Aucun</StatusPill>
                      ),
                  },
                  {
                    header: 'Actions',
                    cell: (o) => (
                      <div className="row-actions">
                        <Button
                          size="sm"
                          blockedReason={
                            writable ?? (readsRoles ? undefined : needs('roles:read', 'La liste des rôles à attribuer'))
                          }
                          onClick={open({ kind: 'roles', operator: o })}
                        >
                          Rôles
                        </Button>
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
                          {link(o).action}
                        </Button>
                        <Toggle
                          operator={o}
                          onEnabled={() => notify(`${o.displayName} réactivé.`)}
                          blockedReason={
                            writable ??
                            (o.id === me.data?.operator.id
                              ? 'Le compte de la session ne se désactive pas ici : un autre détenteur de operators:write peut le faire.'
                              : undefined)
                          }
                          onDisable={open({ kind: 'disable', operator: o })}
                        />
                      </div>
                    ),
                  },
                ]}
              />
            </Card>
          )}
          {pending?.kind === 'create' && (
            <Creation
              onClose={dismiss(pending)}
              onCreated={(o) =>
                notify(`Opérateur ${o.displayName} créé. Le lien d’activation part à ${o.email} ; il vaut 72 heures.`)
              }
            />
          )}
          {pending?.kind === 'roles' && (
            <Roles
              operator={pending.operator}
              self={pending.operator.id === me.data?.operator.id}
              onClose={dismiss(pending)}
              onSaved={() => notify(`Rôles de ${pending.operator.displayName} enregistrés.`)}
            />
          )}
          {pending?.kind === 'disable' && (
            <Disable
              operator={pending.operator}
              onClose={dismiss(pending)}
              onDone={() => notify(`${pending.operator.displayName} désactivé.`)}
            />
          )}
          {pending?.kind === 'link' && (
            <AccessLink
              operator={pending.operator}
              onClose={dismiss(pending)}
              onDone={() => notify(link(pending.operator).sent)}
            />
          )}
        </section>
      ) : (
        <section className="page">
          <Forbidden
            title="Accès aux opérateurs non accordé"
            about="Cet écran liste les comptes du tableau de bord, leurs rôles et l’état de leur second facteur."
            permission="operators:read"
            grantedBy="Propriétaire"
          />
        </section>
      )}
    </>
  )
}

function Creation({ onClose, onCreated }: { onClose: () => void; onCreated: (operator: Operator) => void }) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const form = useId()
  const hint = useId()

  async function create(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() => api.POST('/operators', { body: { email, displayName }, headers: json }))
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
      onCreated(outcome.data)
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
            Créer et inviter
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={create}>
        {refusal}
        <Field
          label="Nom affiché"
          required
          autoComplete="off"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
        <div>
          <Field
            label="Adresse e-mail"
            className="ui-input ui-mono"
            type="email"
            required
            autoComplete="off"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            aria-describedby={describedBy ? `${hint} ${describedBy}` : hint}
          />
          <p className="ui-why" id={hint}>
            Le lien d’activation part à cette adresse.
          </p>
        </div>
        <p>
          L’opérateur naît sans rôle : attribuez-les ensuite par « Rôles » dans la liste. La création est enregistrée
          dans le journal d’audit.
        </p>
      </form>
    </Modal>
  )
}

function Roles({
  operator,
  self,
  onClose,
  onSaved,
}: {
  operator: Operator
  self: boolean
  onClose: () => void
  onSaved: () => void
}) {
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
      onSaved()
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
            Enregistrer
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
        <fieldset className="role-choice role-choice--boxed">
          <legend>Rôles détenus</legend>
          <Choices value={chosen} onChange={setChosen}>
            {roles.data.map((r) => (
              <Choice value={r.id} key={r.id}>
                <span>{r.name}</span> <span className="role-choice__description">{r.description}</span>
              </Choice>
            ))}
          </Choices>
          <p>L’action est enregistrée dans le journal d’audit.</p>
        </fieldset>
      )}
    </Modal>
  )
}

function Disable({ operator, onClose, onDone }: { operator: Operator; onClose: () => void; onDone: () => void }) {
  return (
    <Confirmation
      onDone={() => {
        onDone()
        onClose()
      }}
      invalidate={operatorsQuery.queryKey}
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

function AccessLink({ operator, onClose, onDone }: { operator: Operator; onClose: () => void; onDone: () => void }) {
  const { title, effect } = link(operator)

  return (
    <Confirmation
      onDone={() => {
        onDone()
        onClose()
      }}
      invalidate={operatorsQuery.queryKey}
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

function Toggle({
  operator,
  blockedReason,
  onDisable,
  onEnabled,
}: {
  operator: Operator
  blockedReason?: ReactNode
  onDisable: () => void
  onEnabled: () => void
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
      onEnabled()
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
