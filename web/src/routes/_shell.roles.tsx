import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useEffect, useId, useRef, useState } from 'react'
import { api, json, unreachable } from '../api'
import type { components } from '../api.gen'
import { permissionLabel } from '../permissions'
import { PermissionPicker } from '../picker'
import { rolesQuery } from '../queries'
import { meQuery, Refused, Unauthenticated, useSessionClosed, useShellMe } from '../session'
import {
  answered,
  Button,
  Confirmation,
  DataTable,
  ErrorState,
  Field,
  Forbidden,
  LoadingState,
  Modal,
  needs,
  PageHeader,
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
  const writable = writes ? undefined : needs('roles:write')
  const [pending, setPending] = useState<Pending>()
  const current = useRef(pending)
  current.current = pending
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
      <PageHeader
        crumbs={['Rôles']}
        title="Rôles"
        titleRef={title}
        actions={
          readable && (
            <Button
              size="sm"
              variant="primary"
              blockedReason={writes ? undefined : needs('roles:write', 'La composition d’un rôle')}
              onClick={open({ kind: 'create' })}
            >
              Nouveau rôle
            </Button>
          )
        }
      />
      {readable ? (
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
                if (current.current === pending) {
                  setPending(undefined)
                  setDeleted((n) => n + 1)
                }
              }}
            >
              <p>
                Personne ne détient ce rôle : sa suppression ne retirera aucune permission à aucun opérateur. Elle est
                définitive.
              </p>
            </Confirmation>
          )}
        </section>
      ) : (
        <section className="page">
          <Forbidden
            title="Accès aux rôles non accordé"
            about="Cet écran compose les rôles à partir des permissions de la passerelle et du tableau de bord."
            permission="roles:read"
            grantedBy="Propriétaire"
          />
        </section>
      )}
    </>
  )
}

function View({ role, onClose }: { role: Role; onClose: () => void }) {
  return (
    <Modal title={`Rôle ${role.name}`} wide onClose={onClose} footer={<Button onClick={onClose}>Fermer</Button>}>
      <p>{role.description}</p>
      {/* biome-ignore lint/a11y/noNoninteractiveTabindex: la liste défile ; le clavier doit l’atteindre (WCAG 2.1.1) */}
      <ul className="role-keys" aria-label={`Permissions du rôle ${role.name}`} tabIndex={0}>
        {role.permissions.map((key) => {
          const label = Object.values(permissionLabel(key)).filter(Boolean).join(' — ')
          return (
            <li key={key}>
              {label && `${label} `}
              <span className="role-keys__key">{key}</span>
            </li>
          )
        })}
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
    if (answered(outcome.response)) {
      for (const query of [rolesQuery, meQuery]) {
        void queryClient.invalidateQueries({ queryKey: query.queryKey })
      }
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
        <PermissionPicker keys={keys} chosen={permissions} onChange={setPermissions} />
      </form>
    </Modal>
  )
}
