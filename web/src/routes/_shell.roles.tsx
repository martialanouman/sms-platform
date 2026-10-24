import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { api, json } from '../api'
import type { components } from '../api.gen'
import { Failure, useClosedOn } from '../list'
import { PermissionPicker } from '../picker'
import { rolesQuery } from '../queries'
import { meQuery, useShellMe } from '../session'
import {
  answered,
  Button,
  Card,
  Confirmation,
  DataTable,
  Field,
  Forbidden,
  LoadingState,
  Modal,
  needs,
  PageHeader,
  useSubmission,
  useToast,
} from '../ui'

type Role = components['schemas']['Role']

export const Route = createFileRoute('/_shell/roles')({ component: Roles })

type Pending = { kind: 'create' } | { kind: 'open' | 'delete'; role: Role }

const byDefault = 'Rôle par défaut : réécrit à chaque déploiement, il ne se modifie ni ne se supprime.'

function Roles() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('roles:read')
  const roles = useQuery({ ...rolesQuery, enabled: readable })
  useClosedOn(roles.error)
  const writes = held.includes('roles:write')
  const writable = writes ? undefined : needs('roles:write')
  const [pending, setPending] = useState<Pending>()
  const current = useRef(pending)
  current.current = pending
  const title = useRef<HTMLHeadingElement>(null)
  const notify = useToast()
  const gone = useRef<string>(undefined)

  useEffect(() => {
    if (gone.current && !roles.data?.some((r) => r.id === gone.current)) {
      gone.current = undefined
      title.current?.focus()
    }
  }, [roles.data])

  const open = (p: Pending) => () => setPending(p)
  const dismiss = (p: Pending) => () => setPending((now) => (now === p ? undefined : now))
  const catalog = roles.data?.find((r) => r.name === 'Propriétaire')?.permissions ?? []
  const removable = (r: Role): ReactNode =>
    r.isDefault
      ? byDefault
      : (writable ??
        (r.holders.length > 0
          ? `Détenu par ${r.holders.join(', ')} : il se retire d’abord à ces opérateurs.`
          : undefined))

  return (
    <>
      <PageHeader
        crumbs={['Rôles']}
        title="Rôles"
        titleRef={title}
        meta={
          readable &&
          'Un rôle définit les permissions de la passerelle et du tableau de bord. Les rôles par défaut ne peuvent être modifiés.'
        }
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
            <Failure
              title="Les rôles n’ont pas pu être chargés"
              error={roles.error}
              retry={() => void roles.refetch()}
            />
          ) : (
            <Card
              flush
              footer={
                <span className="ui-muted">
                  {roles.data.length} {roles.data.length === 1 ? 'rôle' : 'rôles'}
                </span>
              }
            >
              <DataTable
                caption="Rôles du tableau de bord"
                rows={roles.data}
                rowKey={(r) => r.id}
                columns={[
                  {
                    header: 'Nom',
                    cell: (r) => (
                      <button type="button" className="ui-link-button" onClick={open({ kind: 'open', role: r })}>
                        {r.name}
                      </button>
                    ),
                  },
                  { header: 'Description', cell: (r) => <span className="ui-muted">{r.description}</span> },
                  {
                    header: 'Type',
                    cell: (r) => <span className="ui-tag">{r.isDefault ? 'Par défaut' : 'Personnalisé'}</span>,
                  },
                  {
                    header: 'Permissions',
                    cell: (r) => <span className="ui-mono">{r.permissions.length}</span>,
                    end: true,
                  },
                  { header: 'Détenteurs', cell: (r) => <span className="ui-mono">{r.holders.length}</span>, end: true },
                  {
                    header: 'Actions',
                    cell: (r) => (
                      <div className="row-actions">
                        <Button
                          size="sm"
                          variant="danger"
                          blockedReason={removable(r)}
                          onClick={open({ kind: 'delete', role: r })}
                        >
                          Supprimer
                        </Button>
                      </div>
                    ),
                  },
                ]}
              />
            </Card>
          )}
          {pending?.kind === 'create' && <RoleForm catalog={catalog} writes={writes} onClose={dismiss(pending)} />}
          {pending?.kind === 'open' && (
            <RoleForm
              role={pending.role}
              catalog={catalog}
              writes={writes}
              removable={removable(pending.role)}
              onClose={dismiss(pending)}
              onDelete={open({ kind: 'delete', role: pending.role })}
            />
          )}
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
                  gone.current = pending.role.id
                  notify(`Rôle ${pending.role.name} supprimé.`)
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

function RoleForm({
  role,
  catalog,
  writes,
  removable,
  onClose,
  onDelete,
}: {
  role?: Role
  catalog: string[]
  writes: boolean
  removable?: ReactNode
  onClose: () => void
  onDelete?: () => void
}) {
  const queryClient = useQueryClient()
  const notify = useToast()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [name, setName] = useState('')
  const [description, setDescription] = useState(role?.description ?? '')
  const [permissions, setPermissions] = useState(role?.permissions ?? [])
  const form = useId()
  const keys = [...catalog].sort()
  const locked = role !== undefined && !writes

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
      notify(role ? `Rôle ${role.name} enregistré.` : `Rôle ${name} créé.`)
      onClose()
    }
  }

  const title = role ? (
    <>
      {role.name} <span className="ui-tag">{role.isDefault ? 'Par défaut' : 'Personnalisé'}</span>
    </>
  ) : (
    'Nouveau rôle'
  )

  if (role?.isDefault) {
    return (
      <Modal title={title} wide onClose={onClose} footer={<Button onClick={onClose}>Fermer</Button>}>
        <p>Les rôles par défaut ne se modifient pas : chaque déploiement les réécrit.</p>
        <p className="ui-muted">{role.description}</p>
        <PermissionPicker keys={keys} chosen={permissions} onChange={() => {}} readOnly streamFollowsTraffic />
      </Modal>
    )
  }

  return (
    <Modal
      title={title}
      wide
      onClose={onClose}
      footer={
        <>
          {role && (
            <span className="ui-modal__aside">
              <Button variant="danger" blockedReason={removable} onClick={onDelete}>
                Supprimer
              </Button>
            </span>
          )}
          <Button onClick={onClose}>Annuler</Button>
          <Button
            variant="primary"
            type="submit"
            form={form}
            loading={waiting}
            blockedReason={locked ? needs('roles:write') : undefined}
          >
            {role ? 'Enregistrer' : 'Créer le rôle'}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        {refusal}
        {!role && (
          <Field
            label="Nom"
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
          readOnly={locked}
          onChange={(e) => setDescription(e.target.value)}
        />
        <PermissionPicker
          keys={keys}
          chosen={permissions}
          onChange={setPermissions}
          readOnly={locked}
          streamFollowsTraffic
        />
      </form>
    </Modal>
  )
}
