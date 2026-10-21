import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useEffect, useId, useRef, useState } from 'react'
import { api, json, unreachable } from '../api'
import type { components } from '../api.gen'
import { rolesQuery } from '../queries'
import { meQuery, Refused, Unauthenticated, useSessionClosed, useShellMe } from '../session'
import {
  Button,
  Confirmation,
  DataTable,
  ErrorState,
  Field,
  LoadingState,
  Modal,
  PageHeader,
  Toolbar,
  useSubmission,
} from '../ui'

type Role = components['schemas']['Role']

export const Route = createFileRoute('/_shell/roles')({ component: Roles })

type Pending = { kind: 'create' } | { kind: 'view' | 'edit' | 'delete'; role: Role }

const byDefault = 'Rôle par défaut : réécrit à chaque déploiement, il ne se modifie ni ne se supprime.'

function Roles() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('roles:read')
  const roles = useQuery({ ...rolesQuery, enabled: readable })
  const closed = useSessionClosed()
  useEffect(() => {
    if (roles.error instanceof Unauthenticated) {
      closed()
    }
  }, [roles.error, closed])
  const writes = held.includes('roles:write')
  const writable = writes ? undefined : 'Cette action demande roles:write.'
  const [pending, setPending] = useState<Pending>()
  const [deleted, setDeleted] = useState(0)
  const title = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    if (deleted) {
      title.current?.focus()
    }
  }, [deleted])

  const open = (p: Pending) => () => setPending(p)
  const dismiss = (p: Pending) => () => setPending((now) => (now === p ? undefined : now))
  const catalog = roles.data?.find((r) => r.name === 'Propriétaire')?.permissions ?? []

  return (
    <>
      <PageHeader title="Rôles" titleRef={title} />
      {readable ? (
        <>
          <Toolbar>
            <Button
              size="sm"
              variant="primary"
              blockedReason={writes ? undefined : 'La composition d’un rôle demande roles:write.'}
              onClick={open({ kind: 'create' })}
            >
              Nouveau rôle
            </Button>
          </Toolbar>
          <section className="page">
            {roles.isPending ? (
              <LoadingState label="Chargement des rôles…" rows={3} />
            ) : roles.error ? (
              <ErrorState as="h2" title="Les rôles n’ont pas pu être chargés" onRetry={() => void roles.refetch()}>
                {roles.error instanceof Refused ? roles.error.message : unreachable}
              </ErrorState>
            ) : (
              <DataTable
                caption="Rôles du tableau de bord"
                rows={roles.data}
                rowKey={(r) => r.id}
                columns={[
                  { header: 'Nom', cell: (r) => r.name },
                  { header: 'Description', cell: (r) => r.description },
                  { header: 'Permissions', cell: (r) => r.permissions.length, end: true },
                  { header: 'Détenteurs', cell: (r) => r.holders.length, end: true },
                  {
                    header: 'Actions',
                    cell: (r) => (
                      <div className="row-actions">
                        <Button size="sm" onClick={open({ kind: 'view', role: r })}>
                          Voir les permissions
                        </Button>
                        <Button
                          size="sm"
                          blockedReason={r.isDefault ? byDefault : writable}
                          onClick={open({ kind: 'edit', role: r })}
                        >
                          Modifier
                        </Button>
                        <Button
                          size="sm"
                          variant="danger"
                          blockedReason={
                            r.isDefault
                              ? byDefault
                              : (writable ??
                                (r.holders.length > 0
                                  ? `Détenu par ${r.holders.join(', ')} : il se retire d’abord à ces opérateurs.`
                                  : undefined))
                          }
                          onClick={open({ kind: 'delete', role: r })}
                        >
                          Supprimer
                        </Button>
                      </div>
                    ),
                  },
                ]}
              />
            )}
            {pending?.kind === 'view' && <View role={pending.role} onClose={dismiss(pending)} />}
            {pending?.kind === 'create' && <Editor catalog={catalog} onClose={dismiss(pending)} />}
            {pending?.kind === 'edit' && <Editor role={pending.role} catalog={catalog} onClose={dismiss(pending)} />}
            {pending?.kind === 'delete' && (
              <Confirmation
                title={`Supprimer le rôle ${pending.role.name} ?`}
                action="Supprimer"
                danger
                invalidate={rolesQuery.queryKey}
                call={() =>
                  api.DELETE('/roles/{roleId}', { params: { path: { roleId: pending.role.id } }, headers: json })
                }
                onClose={dismiss(pending)}
                onDone={() => {
                  dismiss(pending)()
                  setDeleted((n) => n + 1)
                }}
              >
                <p>
                  Personne ne détient ce rôle : sa suppression ne retirera aucune permission à aucun opérateur. Elle est
                  définitive.
                </p>
              </Confirmation>
            )}
          </section>
        </>
      ) : (
        <section className="page">
          <p className="auth__intro">Il faut la permission roles:read pour lire les rôles.</p>
        </section>
      )}
    </>
  )
}

function View({ role, onClose }: { role: Role; onClose: () => void }) {
  return (
    <Modal title={`Rôle ${role.name}`} onClose={onClose} footer={<Button onClick={onClose}>Fermer</Button>}>
      <p>{role.description}</p>
      <ul className="role-keys">
        {role.permissions.map((key) => (
          <li key={key}>{key}</li>
        ))}
      </ul>
    </Modal>
  )
}

function Editor({ role, catalog, onClose }: { role?: Role; catalog: string[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [name, setName] = useState('')
  const [description, setDescription] = useState(role?.description ?? '')
  const [permissions, setPermissions] = useState(role?.permissions ?? [])
  const form = useId()
  const keys = [...catalog].sort()
  const resources = [...new Set(keys.map((key) => key.split(':')[0] ?? key))]

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      role
        ? api.PATCH('/roles/{roleId}', {
            params: { path: { roleId: role.id } },
            body: { description, permissions },
            headers: json,
          })
        : api.POST('/roles', { body: { name, description, permissions }, headers: json }),
    )
    for (const query of [rolesQuery, meQuery]) {
      void queryClient.invalidateQueries({ queryKey: query.queryKey })
    }
    if (outcome.failure === undefined) {
      onClose()
    }
  }

  return (
    <Modal
      title={role ? `Modifier ${role.name}` : 'Nouveau rôle'}
      wide
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            {role ? 'Enregistrer le rôle' : 'Créer le rôle'}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        <p>
          {role
            ? 'Ses détenteurs reçoivent aussitôt les clés cochées.'
            : 'Le rôle accorde exactement les clés cochées.'}{' '}
          L’action est enregistrée dans le journal d’audit.
        </p>
        {refusal}
        {!role && (
          <Field
            label="Nom"
            className="ui-input ui-input--mono"
            required
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-describedby={describedBy}
          />
        )}
        <Field
          label="Description"
          maxLength={500}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <div className="permission-picker">
          {resources.map((resource) => (
            <fieldset className="role-choice" key={resource}>
              <legend>{resource}</legend>
              {keys
                .filter((key) => key.startsWith(`${resource}:`))
                .map((key) => (
                  <label className="role-choice__option" key={key}>
                    <input
                      type="checkbox"
                      checked={permissions.includes(key)}
                      onChange={(e) =>
                        setPermissions(e.target.checked ? [...permissions, key] : permissions.filter((p) => p !== key))
                      }
                    />
                    <span className="role-keys__key">{key}</span>
                  </label>
                ))}
            </fieldset>
          ))}
        </div>
      </form>
    </Modal>
  )
}
