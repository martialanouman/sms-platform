import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useId, useState } from 'react'
import type { components } from '../admin.gen'
import { json } from '../api'
import { gw } from '../gw'
import { Failure, type Paged, Pager, paged, useClosedOn } from '../list'
import { read } from '../queries'
import { useShellMe } from '../session'
import {
  Button,
  Confirmation,
  DataTable,
  EmptyState,
  Field,
  Forbidden,
  LoadingState,
  Modal,
  PageHeader,
  Select,
  Toolbar,
  useSubmission,
} from '../ui'

type Group = components['schemas']['CustomerGroup']
type Status = Group['status']
type GroupsSearch = Paged & { status?: Status }

export const Route = createFileRoute('/_shell/groups')({
  validateSearch: (s: Record<string, unknown>): GroupsSearch => ({
    ...paged(s),
    status: s.status === 'active' || s.status === 'archived' ? s.status : undefined,
  }),
  component: Groups,
})

const groupsKey = ['customer-groups']

const statusLabel: Record<Status, string> = { active: 'Actif', archived: 'Archivé' }

const filters = [
  { value: '', label: 'Tous' },
  { value: 'active', label: 'Actifs' },
  { value: 'archived', label: 'Archivés' },
]

type Pending = { kind: 'create' } | { kind: 'edit' | 'status' | 'delete'; group: Group }

function Groups() {
  const me = useShellMe()
  const held = me.data?.permissions ?? []
  const readable = held.includes('customer-groups:read')
  const blocked = held.includes('customer-groups:write') ? undefined : 'Cette action demande customer-groups:write.'
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const list = useQuery({
    queryKey: [...groupsKey, search],
    queryFn: () =>
      read(
        gw.GET('/admin/customer-groups', { params: { query: { status: search.status, cursor: search.cursor } } }),
        'customer-groups',
      ),
    enabled: readable,
  })
  useClosedOn(list.error)
  const [pending, setPending] = useState<Pending>()
  const close = () => setPending(undefined)

  return (
    <>
      <PageHeader title="Groupes de clients" />
      {readable ? (
        <>
          <Toolbar
            filters={
              <Select
                label="Statut"
                items={filters}
                value={search.status ?? ''}
                onChange={(status) =>
                  void navigate({ search: { status: status === '' ? undefined : (status as Status) } })
                }
              />
            }
          >
            <Button size="sm" variant="primary" blockedReason={blocked} onClick={() => setPending({ kind: 'create' })}>
              Nouveau groupe
            </Button>
          </Toolbar>
          <section className="page">
            {list.isPending ? (
              <LoadingState label="Chargement des groupes…" rows={3} />
            ) : list.error ? (
              <Failure
                title="Les groupes n’ont pas pu être chargés"
                error={list.error}
                retry={() => void list.refetch()}
              />
            ) : list.data.data.length === 0 ? (
              <EmptyState title="Aucun groupe" />
            ) : (
              <>
                <DataTable<Group>
                  caption="Groupes de clients"
                  rows={list.data.data}
                  rowKey={(g) => g.id}
                  columns={[
                    { header: 'Nom', cell: (g) => g.name },
                    { header: 'Description', cell: (g) => g.description ?? '—' },
                    { header: 'Statut', cell: (g) => statusLabel[g.status] },
                    { header: 'Clients', cell: (g) => g.member_count },
                    {
                      header: 'Actions',
                      cell: (g) => (
                        <div className="row-actions">
                          <Button
                            size="sm"
                            blockedReason={blocked}
                            onClick={() => setPending({ kind: 'edit', group: g })}
                          >
                            Modifier
                          </Button>
                          <Button
                            size="sm"
                            blockedReason={blocked}
                            onClick={() => setPending({ kind: 'status', group: g })}
                          >
                            {g.status === 'active' ? 'Archiver' : 'Réactiver'}
                          </Button>
                          <Button
                            size="sm"
                            blockedReason={blocked}
                            onClick={() => setPending({ kind: 'delete', group: g })}
                          >
                            Supprimer
                          </Button>
                        </div>
                      ),
                    },
                  ]}
                />
                <Pager search={search} next={list.data.next_cursor} go={(next) => void navigate({ search: next })} />
              </>
            )}
            {pending?.kind === 'create' && <GroupForm onClose={close} />}
            {pending?.kind === 'edit' && <GroupForm group={pending.group} onClose={close} />}
            {pending?.kind === 'status' && <StatusChange group={pending.group} onClose={close} />}
            {pending?.kind === 'delete' && <Deletion group={pending.group} onClose={close} />}
          </section>
        </>
      ) : (
        <section className="page">
          <Forbidden
            title="Accès aux groupes de clients non accordé"
            about="Cet écran liste les groupes qui rangent les clients pour le filtrage et le reporting."
            permission="customer-groups:read"
            grantedBy="Exploitation"
          />
        </section>
      )}
    </>
  )
}

function GroupForm({ group, onClose }: { group?: Group; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const [name, setName] = useState(group?.name ?? '')
  const [description, setDescription] = useState(group?.description ?? '')
  const form = useId()

  async function save(event: FormEvent) {
    event.preventDefault()
    const typed = description === '' ? null : description
    const outcome = group
      ? await submit(() =>
          gw.PATCH('/admin/customer-groups/{id}', {
            params: { path: { id: group.id } },
            body: {
              ...(name !== group.name && { name }),
              ...(typed !== group.description && { description: typed }),
            },
            headers: json,
          }),
        )
      : await submit(() =>
          gw.POST('/admin/customer-groups', {
            body: { name, ...(typed !== null && { description: typed }) },
            headers: json,
          }),
        )
    if (outcome.response?.status === 409) {
      fail('Un groupe porte déjà ce nom.')
      return
    }
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: groupsKey })
      onClose()
    }
  }

  return (
    <Modal
      title={group ? `Modifier le groupe ${group.name}` : 'Nouveau groupe'}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            {group ? 'Enregistrer' : 'Créer le groupe'}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        <p>Un groupe range des clients pour le filtrage et le reporting ; il ne porte ni solde, ni quota, ni règle.</p>
        {refusal}
        <Field
          label="Nom"
          required
          maxLength={200}
          autoComplete="off"
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-describedby={describedBy}
        />
        <Field
          label="Description"
          maxLength={1000}
          autoComplete="off"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </form>
    </Modal>
  )
}

function StatusChange({ group, onClose }: { group: Group; onClose: () => void }) {
  const archive = group.status === 'active'
  const action = archive ? 'Archiver' : 'Réactiver'
  return (
    <Confirmation
      title={`${action} le groupe ${group.name}`}
      action={action}
      invalidate={groupsKey}
      onClose={onClose}
      call={() =>
        gw.PATCH('/admin/customer-groups/{id}', {
          params: { path: { id: group.id } },
          body: { status: archive ? 'archived' : 'active' },
          headers: json,
        })
      }
    >
      <p>
        {archive
          ? 'Un groupe archivé n’est plus proposé à l’affectation ; ses clients le gardent.'
          : 'Le groupe est de nouveau proposé à l’affectation.'}
      </p>
    </Confirmation>
  )
}

function Deletion({ group, onClose }: { group: Group; onClose: () => void }) {
  return (
    <Confirmation
      title={`Supprimer le groupe ${group.name}`}
      action="Supprimer"
      danger
      invalidate={groupsKey}
      onClose={onClose}
      call={() => gw.DELETE('/admin/customer-groups/{id}', { params: { path: { id: group.id } } })}
    >
      <p>Ses clients ({group.member_count}) sont détachés du groupe, pas supprimés.</p>
    </Confirmation>
  )
}
