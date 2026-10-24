import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { type FormEvent, useId, useState } from 'react'
import type { components } from './admin.gen'
import { gw } from './gw'
import { read } from './queries'
import { useShellMe } from './session'
import { Button, Field, Modal, needs, Select, StatusPill, type Tone, useSubmission, useToast } from './ui'

export type Customer = components['schemas']['Customer']
export type Group = components['schemas']['CustomerGroup']
type Ages = components['schemas']['MaxAgeOverride']

export const customersKey = ['customers']

export const customerQuery = (id: string) =>
  queryOptions({
    queryKey: [...customersKey, id],
    queryFn: () => read(gw.GET('/admin/customers/{id}', { params: { path: { id } } }), 'customer'),
  })

export const groupsKey = ['customer-groups']

export const groupsBlocked = needs('customer-groups:read', 'Le groupe')

const customerStatus: Record<Customer['status'], { label: string; tone: Tone }> = {
  active: { label: 'Actif', tone: 'ok' },
  suspended: { label: 'Suspendu', tone: 'warn' },
  closed: { label: 'Fermé', tone: 'neutral' },
}

export function Status({ status }: { status: Customer['status'] }) {
  return <StatusPill tone={customerStatus[status].tone}>{customerStatus[status].label}</StatusPill>
}

export const statusOf = (v: unknown) => (v === 'active' || v === 'suspended' || v === 'closed' ? v : undefined)

export function GroupFilter({ value, onChange }: { value?: string; onChange: (id: string) => void }) {
  const groups = useGroups()
  return (
    <Select
      label="Groupe"
      items={[{ value: '', label: 'Tous' }, ...groups.list.map((g) => ({ value: g.id, label: groupLabel(g) }))]}
      value={value ?? ''}
      onChange={onChange}
      blockedReason={groups.blocked}
    />
  )
}

export const statusFilters = [
  { value: '', label: 'Tous' },
  { value: 'active', label: 'Actifs' },
  { value: 'suspended', label: 'Suspendus' },
  { value: 'closed', label: 'Fermés' },
]

export function useGroups() {
  const me = useShellMe()
  const readable = (me.data?.permissions ?? []).includes('customer-groups:read')
  const groups = useQuery({
    queryKey: [...groupsKey, 'directory'],
    queryFn: () => read(gw.GET('/admin/customer-groups', { params: { query: { limit: 500 } } }), 'customer-groups'),
    enabled: readable,
  })
  const list = [...(groups.data?.data ?? [])].sort((a, b) => a.name.localeCompare(b.name, 'fr'))
  const name = (id: string | null) => (id ? (list.find((g) => g.id === id)?.name ?? id) : '—')
  const archived = (id: string) => list.find((g) => g.id === id)?.status === 'archived'
  return { list, name, archived, blocked: readable ? undefined : groupsBlocked }
}

export function GroupName({ id, link }: { id: string; link?: boolean }) {
  const groups = useGroups()
  if (groups.blocked) {
    return <span className="ui-muted">non lisible</span>
  }
  const name = (
    <>
      {groups.name(id)}
      {groups.archived(id) && <span className="ui-muted"> (archivé)</span>}
    </>
  )
  return link ? (
    <Link to="/customers" search={{ group_id: id }}>
      {name}
    </Link>
  ) : (
    name
  )
}

export const groupLabel = (g: Group) => (g.status === 'archived' ? `${g.name} (archivé)` : g.name)

export const ageFields = [
  { key: 'otp', label: 'OTP', max: 600 },
  { key: 'transactional', label: 'Transactionnel', max: 86400 },
  { key: 'marketing', label: 'Marketing', max: 259200 },
] as const

const none: Ages = { otp: null, transactional: null, marketing: null }

export function CustomerForm({ customer, onClose }: { customer?: Customer; onClose: () => void }) {
  const queryClient = useQueryClient()
  const { list, blocked } = useGroups()
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const notify = useToast()
  const [name, setName] = useState(customer?.name ?? '')
  const [group, setGroup] = useState(customer?.group_id ?? '')
  const loaded = customer?.max_age_override ?? none
  const [ages, setAges] = useState(
    Object.fromEntries(ageFields.map((f) => [f.key, loaded[f.key]?.toString() ?? ''])) as Record<keyof Ages, string>,
  )
  const form = useId()
  const offered = list.filter((g) => g.status === 'active' || g.id === customer?.group_id)

  async function save(event: FormEvent) {
    event.preventDefault()
    const groupID = group === '' ? null : group
    const typed = Object.fromEntries(
      ageFields.map((f) => [f.key, ages[f.key] === '' ? null : Number(ages[f.key])]),
    ) as Ages
    const agesChanged = ageFields.some((f) => typed[f.key] !== loaded[f.key])
    const outcome = customer
      ? await submit(() =>
          gw.PATCH('/admin/customers/{id}', {
            params: { path: { id: customer.id } },
            body: {
              ...(name !== customer.name && { name }),
              ...(groupID !== customer.group_id && { group_id: groupID }),
              ...(agesChanged && { max_age_override: typed }),
            },
          }),
        )
      : await submit(() =>
          gw.POST('/admin/customers', {
            body: {
              name,
              ...(groupID !== null && { group_id: groupID }),
              ...(agesChanged && { max_age_override: typed }),
            },
          }),
        )
    if (outcome.response?.status === 422) {
      fail('Ce groupe n’existe plus.')
      return
    }
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: customersKey })
      notify(`Client ${name} ${customer ? 'modifié' : 'créé'}.`)
      onClose()
    }
  }

  return (
    <Modal
      title={customer ? `Modifier le client ${customer.name}` : 'Nouveau client'}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            {customer ? 'Enregistrer' : 'Créer le client'}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
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
        <Select
          label="Groupe"
          items={[{ value: '', label: 'Aucun groupe' }, ...offered.map((g) => ({ value: g.id, label: groupLabel(g) }))]}
          value={group}
          onChange={setGroup}
          blockedReason={blocked}
        />
        <p>
          Âge maximal d’un message, en secondes ; vide, le plafond de la plateforme s’applique (10 min, 24 h, 72 h). Il
          ne peut que l’abaisser.
        </p>
        {ageFields.map((f) => (
          <Field
            key={f.key}
            label={`${f.label} (s)`}
            type="number"
            min={1}
            max={f.max}
            step={1}
            placeholder={String(f.max)}
            value={ages[f.key]}
            onChange={(e) => setAges({ ...ages, [f.key]: e.target.value })}
          />
        ))}
      </form>
    </Modal>
  )
}
