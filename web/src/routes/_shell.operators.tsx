import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { api, json, send, unreachable } from '../api'
import type { components } from '../api.gen'
import { meQuery, useShellMe } from '../session'
import { Field, useSubmission } from '../ui'

type Operator = components['schemas']['Operator']

export const Route = createFileRoute('/_shell/operators')({ component: Operators })

const operatorsQuery = {
  queryKey: ['operators'],
  queryFn: async () => {
    const { data } = await api.GET('/operators')
    if (!data) {
      throw new Error('operators')
    }
    return data
  },
}

const linkKinds = { activation: 'Activation', reset: 'Réinitialisation' } as const
const linkStates = { queued: 'en file', sent: 'envoyée', failed: 'en échec' } as const

function Explained({ reason, children }: { reason: string; children: (describedBy?: string) => ReactNode }) {
  const id = useId()
  if (!reason) {
    return children()
  }
  return (
    <>
      {children(id)}
      <span className="table__hint" id={id}>
        {reason}
      </span>
    </>
  )
}

function Operators() {
  const me = useShellMe()
  const operators = useQuery(operatorsQuery)
  const held = me.data?.permissions ?? []
  const writable = held.includes('operators:write') ? '' : 'Il faut la permission operators:write.'
  const [editing, setEditing] = useState<{ operator: Operator; opener: HTMLButtonElement }>()

  useEffect(() => {
    document.title = 'Opérateurs — Tableau de bord'
  }, [])

  return (
    <section className="page">
      <h1 className="page__title">Opérateurs</h1>
      <Creation refusal={writable} />
      {operators.error && (
        <div className="shell__failure">
          <p className="auth__refusal" role="alert">
            {unreachable}
          </p>
          <button className="ui-button ui-button--secondary" type="button" onClick={() => void operators.refetch()}>
            Réessayer
          </button>
        </div>
      )}
      {operators.isPending && <p className="auth__intro">Chargement des opérateurs…</p>}
      {operators.data?.length === 0 && <p className="auth__intro">Aucun opérateur.</p>}
      {operators.data && operators.data.length > 0 && (
        <table className="table">
          <thead>
            <tr>
              <th scope="col">Nom</th>
              <th scope="col">Adresse e-mail</th>
              <th scope="col">Statut</th>
              <th scope="col">Rôles</th>
              <th scope="col">Second facteur</th>
              <th scope="col">Lien d'accès</th>
              <th scope="col">Actions</th>
            </tr>
          </thead>
          <tbody>
            {operators.data.map((o) => (
              <Row
                key={o.id}
                operator={o}
                self={o.id === me.data?.operator.id}
                writable={writable}
                readsRoles={held.includes('roles:read')}
                onRoles={(opener) => setEditing({ operator: o, opener })}
              />
            ))}
          </tbody>
        </table>
      )}
      {editing && (
        <RolesDialog
          operator={editing.operator}
          self={editing.operator.id === me.data?.operator.id}
          onClose={() => {
            editing.opener.focus()
            setEditing(undefined)
          }}
        />
      )}
    </section>
  )
}

function Creation({ refusal }: { refusal: string }) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal: failure, describedBy } = useSubmission()
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')

  async function create(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() => api.POST('/operators', { body: { email, displayName }, headers: json }))
    if (outcome.failure === undefined) {
      setEmail('')
      setDisplayName('')
      await queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
    }
  }

  return (
    <form className="inline-form" aria-label="Nouvel opérateur" onSubmit={create}>
      {failure}
      <Field
        label="Adresse e-mail"
        type="email"
        required
        disabled={!!refusal}
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        aria-describedby={describedBy}
      />
      <Field
        label="Nom"
        required
        disabled={!!refusal}
        value={displayName}
        onChange={(e) => setDisplayName(e.target.value)}
      />
      <Explained reason={refusal}>
        {(hint) => (
          <button
            className="ui-button ui-button--primary"
            type="submit"
            disabled={!!refusal || waiting}
            aria-describedby={hint}
          >
            Créer
          </button>
        )}
      </Explained>
    </form>
  )
}

function Row({
  operator: o,
  self,
  writable,
  readsRoles,
  onRoles,
}: {
  operator: Operator
  self: boolean
  writable: string
  readsRoles: boolean
  onRoles: (opener: HTMLButtonElement) => void
}) {
  const queryClient = useQueryClient()
  const [outcome, setOutcome] = useState<{ refusal?: string; note?: string }>({})
  const active = o.status === 'active'

  async function act(call: Parameters<typeof send>[0], note?: string) {
    setOutcome({})
    const { failure } = await send(call)
    if (failure !== undefined) {
      setOutcome({ refusal: failure })
      return
    }
    setOutcome({ note })
    await queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
  }

  return (
    <tr>
      <th scope="row">{o.displayName}</th>
      <td>{o.email}</td>
      <td>{active ? 'Actif' : 'Désactivé'}</td>
      <td>{o.roles.length > 0 ? o.roles.map((r) => r.name).join(', ') : 'Aucun'}</td>
      <td>{o.secondFactorEnrolled ? 'Oui' : 'Non'}</td>
      <td>{o.accessLink ? `${linkKinds[o.accessLink.kind]} ${linkStates[o.accessLink.state]}` : '—'}</td>
      <td>
        <div className="table__actions">
          <Explained reason={writable || (readsRoles ? '' : 'Il faut la permission roles:read.')}>
            {(hint) => (
              <button
                className="ui-button ui-button--secondary"
                type="button"
                disabled={!!hint}
                aria-describedby={hint}
                onClick={(e) => onRoles(e.currentTarget)}
              >
                Rôles
              </button>
            )}
          </Explained>
          <Explained reason={writable || (active ? '' : 'Aucun lien ne part vers un compte désactivé.')}>
            {(hint) => (
              <button
                className="ui-button ui-button--secondary"
                type="button"
                disabled={!!hint}
                aria-describedby={hint}
                onClick={() =>
                  void act(
                    () =>
                      api.POST('/operators/{operatorId}/access-link', {
                        params: { path: { operatorId: o.id } },
                        headers: json,
                      }),
                    'Lien demandé.',
                  )
                }
              >
                Envoyer un lien
              </button>
            )}
          </Explained>
          <Explained reason={writable || (self && active ? 'Vous ne pouvez pas vous désactiver.' : '')}>
            {(hint) => (
              <button
                className="ui-button ui-button--secondary"
                type="button"
                disabled={!!hint}
                aria-describedby={hint}
                onClick={() =>
                  void act(() =>
                    api.PATCH('/operators/{operatorId}', {
                      params: { path: { operatorId: o.id } },
                      body: { status: active ? 'disabled' : 'active' },
                      headers: json,
                    }),
                  )
                }
              >
                {active ? 'Désactiver' : 'Réactiver'}
              </button>
            )}
          </Explained>
        </div>
        {outcome.refusal && (
          <p className="table__refusal" role="alert">
            {outcome.refusal}
          </p>
        )}
        {outcome.note && (
          <p className="table__note" role="status">
            {outcome.note}
          </p>
        )}
      </td>
    </tr>
  )
}

function RolesDialog({ operator, self, onClose }: { operator: Operator; self: boolean; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const title = useId()
  const queryClient = useQueryClient()
  const roles = useQuery({
    queryKey: ['roles'],
    queryFn: async () => {
      const { data } = await api.GET('/roles')
      if (!data) {
        throw new Error('roles')
      }
      return data
    },
  })
  const [chosen, setChosen] = useState(operator.roles.map((r) => r.id))
  const { submit, waiting, refusal } = useSubmission()

  const closed = useRef(onClose)
  closed.current = onClose

  useEffect(() => {
    const element = dialog.current
    const close = () => closed.current()
    element?.showModal()
    element?.addEventListener('close', close)
    return () => element?.removeEventListener('close', close)
  }, [])

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      api.PUT('/operators/{operatorId}/roles', {
        params: { path: { operatorId: operator.id } },
        body: { roleIds: chosen },
        headers: json,
      }),
    )
    if (outcome.failure === undefined) {
      await queryClient.invalidateQueries({ queryKey: operatorsQuery.queryKey })
      if (self) {
        await queryClient.invalidateQueries({ queryKey: meQuery.queryKey })
      }
      dialog.current?.close()
    }
  }

  return (
    <dialog ref={dialog} className="dialog" aria-labelledby={title}>
      <form className="dialog__form" onSubmit={save}>
        <h2 className="dialog__title" id={title}>
          Rôles de {operator.displayName}
        </h2>
        {refusal}
        {roles.error && (
          <p className="auth__refusal" role="alert">
            {unreachable}
          </p>
        )}
        {roles.data && (
          <fieldset className="dialog__choices">
            <legend className="ui-field__label">Rôles détenus</legend>
            {roles.data.map((r) => (
              <label key={r.id} className="dialog__choice">
                <input
                  type="checkbox"
                  checked={chosen.includes(r.id)}
                  onChange={(e) => setChosen(e.target.checked ? [...chosen, r.id] : chosen.filter((id) => id !== r.id))}
                />
                <span>
                  {r.name}
                  <span className="dialog__description"> — {r.description}</span>
                </span>
              </label>
            ))}
          </fieldset>
        )}
        <div className="dialog__actions">
          <button className="ui-button ui-button--secondary" type="button" onClick={() => dialog.current?.close()}>
            Annuler
          </button>
          <button className="ui-button ui-button--primary" type="submit" disabled={waiting || !roles.data}>
            Enregistrer
          </button>
        </div>
      </form>
    </dialog>
  )
}
