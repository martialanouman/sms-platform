import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { atMinute } from '../audit'
import { empty, explained, json, me, once, renderAt, stubFetch, violations } from '../test'

const reader = { ...me, permissions: ['customer-groups:read'] }
const writer = { ...me, permissions: ['customer-groups:read', 'customer-groups:write'] }
const banks = {
  id: '01a16026-0000-7000-8000-000000000001',
  name: 'Banques',
  description: 'Établissements financiers',
  status: 'active',
  member_count: 3,
  created_by: me.operator.id,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
}
const telcos = {
  ...banks,
  id: '01a16026-0000-7000-8000-000000000002',
  name: 'Télécoms',
  description: null,
  status: 'archived',
  member_count: 0,
}
const page = (data: unknown[], next: string | null = null, total = data.length) =>
  json(200, { data, next_cursor: next, total })

async function row(name: string) {
  return within((await screen.findByRole('rowheader', { name })).closest('tr') as HTMLTableRowElement)
}

async function cells(name: string) {
  return (await row(name)).getAllByRole('cell').map((c) => c.textContent)
}

test('the rail offers Groups under Clients to whoever reads customer groups', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => page([]),
  })
  renderAt('/groups')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  const clients = await within(rail).findByRole('list', { name: 'Clients' })
  expect(within(clients).getByRole('link', { name: 'Groupes' })).toBeDefined()
})

test('without customer-groups:read the screen says why and asks nothing', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['audit:read'] }) })
  renderAt('/groups')
  expect(await screen.findByRole('heading', { name: 'Accès aux groupes de clients non accordé' })).toBeDefined()
  expect(calls.some((r) => new URL(r.url).pathname.startsWith('/api/gw/'))).toBe(false)
})

test('the groups are listed with their status and member count', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => page([banks, telcos], null, 17),
  })
  const { container } = renderAt('/groups')
  expect(await screen.findByRole('table', { name: 'Groupes de clients' })).toBeDefined()
  expect(document.title).toBe('Groupes de clients — Tableau de bord')
  expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual([
    'Nom',
    'Description',
    'Statut',
    'Clients',
    'Créé le',
    'Actions',
  ])
  expect((await cells('Banques')).slice(0, 4)).toEqual([
    'Établissements financiers',
    'Actif',
    '3',
    atMinute(banks.created_at),
  ])
  expect(screen.getByRole('link', { name: '3' }).getAttribute('href')).toBe(`/customers?group_id=${banks.id}`)
  expect((await cells('Télécoms')).slice(0, 3)).toEqual(['—', 'Archivé', '0'])
  expect((await row('Banques')).getByText('Actif').className).toBe('ui-pill ui-pill--ok')
  expect((await row('Télécoms')).getByText('Archivé').className).toBe('ui-pill ui-pill--neutral')
  expect((await row('Banques')).getByText(atMinute(banks.created_at)).className).toContain('ui-mono')
  expect(atMinute(banks.created_at)).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/)
  expect((await row('Banques')).getByRole('button', { name: 'Supprimer' }).className).toContain('ui-button--danger')
  expect((await row('Banques')).getByRole('button', { name: 'Modifier' }).closest('.row-actions')).not.toBeNull()
  const table = screen.getByRole('table', { name: 'Groupes de clients' })
  expect(screen.getByText('2 entrées affichées sur 17').closest('.ui-card')).toBe(table.closest('.ui-card'))
  expect(await violations(container)).toEqual([])
})

test('the header says what a group is and offers to create one', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
  })
  renderAt('/groups')
  const title = await screen.findByRole('heading', { level: 1, name: 'Groupes de clients' })
  const header = title.closest('.page-header') as HTMLElement
  expect(header.textContent).toContain(
    'Un groupe range des clients pour le filtrage et le reporting ; il ne porte ni solde, ni quota, ni règle.',
  )
  expect(within(header).getByRole('button', { name: 'Nouveau groupe' })).toBeDefined()
})

test('no group at all says how to create one, and none under the filter asks to widen it', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([]),
  })
  const { unmount } = renderAt('/groups')
  expect(await screen.findByText('Aucun groupe')).toBeDefined()
  expect(screen.getByText('Créez le premier groupe avec « Nouveau groupe ».')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Retirer les filtres' })).toBeNull()
  unmount()
  const { router } = renderAt('/groups?status=archived')
  expect(await screen.findByText('Aucun résultat')).toBeDefined()
  expect(screen.getByText('Élargissez les filtres.')).toBeDefined()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retirer les filtres' }))
  await waitFor(() => expect(router.state.location.search).toEqual({}))
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Groupes de clients' })),
  )
})

test('the status filter and the next page travel in the address', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': (r) =>
      new URL(r.url).searchParams.get('cursor') ? page([telcos]) : page([banks], 'c1'),
  })
  const user = userEvent.setup()
  const { router } = renderAt('/groups')
  await screen.findByText('Banques')

  expect(screen.getByRole('combobox').textContent).toBe('Statut : Tous')
  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Archivés' }))
  await waitFor(() => expect(router.state.location.search).toEqual({ status: 'archived' }))
  expect(screen.getByRole('combobox').textContent).toBe('Statut : Archivés')
  await waitFor(() => expect(calls.some((r) => new URL(r.url).searchParams.get('status') === 'archived')).toBe(true))

  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Tous' }))
  await waitFor(() => expect(router.state.location.search).toEqual({}))
  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Archivés' }))
  await waitFor(() => expect(router.state.location.search).toEqual({ status: 'archived' }))

  await user.click(await screen.findByRole('button', { name: 'Suivant' }))
  await screen.findByText('Télécoms')
  const last = new URL(calls.at(-1)?.url ?? '')
  expect([last.searchParams.get('status'), last.searchParams.get('cursor')]).toEqual(['archived', 'c1'])
})

test('a group is created with its name, and an empty description is not sent', async () => {
  let listed = 0
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => (listed++ === 0 ? page([]) : page([banks])),
    'POST /api/gw/admin/customer-groups': () => json(201, banks),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click(await screen.findByRole('button', { name: 'Nouveau groupe' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau groupe' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Banques')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le groupe' }))

  await screen.findByRole('rowheader', { name: 'Banques' })
  const post = calls.find((r) => r.method === 'POST' && r.url.endsWith('/admin/customer-groups'))
  expect(await post?.json()).toEqual({ name: 'Banques' })
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('a name already taken is said in French in the dialog', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([]),
    'POST /api/gw/admin/customer-groups': () =>
      json(409, {
        code: 'conflict',
        message: 'already taken',
        errors: [{ field: 'body.name', message: 'already taken' }],
      }),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click(await screen.findByRole('button', { name: 'Nouveau groupe' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau groupe' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Banques')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le groupe' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Un groupe porte déjà ce nom.')
})

test('an edit sends only what changed, and an emptied description as null', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
    'PATCH /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000001': () => json(200, banks),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click((await row('Banques')).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le groupe Banques' })
  await user.clear(within(dialog).getByLabelText('Description'))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))

  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({ description: null })
})

test('a renamed group sends its name alone', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
    'PATCH /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000001': () => json(200, banks),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click((await row('Banques')).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le groupe Banques' })
  await user.type(within(dialog).getByLabelText('Nom'), ' et assurances')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))

  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({ name: 'Banques et assurances' })
})

test('a refused edit keeps the dialog open with the reason', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
    'PATCH /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000001': () =>
      json(422, { code: 'validation_error', message: 'validation failed' }),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click((await row('Banques')).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le groupe Banques' })
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('validation failed')
})

test('archiving and reactivating set the status', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks, telcos]),
    'PATCH /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000001': () => json(200, banks),
    'PATCH /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000002': () => json(200, telcos),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click((await row('Banques')).getByRole('button', { name: 'Archiver' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Archiver' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await user.click((await row('Télécoms')).getByRole('button', { name: 'Réactiver' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Réactiver' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

  const patches = calls.filter((r) => r.method === 'PATCH')
  expect(await Promise.all(patches.map((r) => r.json()))).toEqual([{ status: 'archived' }, { status: 'active' }])
  await waitFor(() =>
    expect(calls.filter((r) => r.method === 'GET' && r.url.includes('customer-groups')).length).toBe(3),
  )
  expect(patches.map((r) => new URL(r.url).pathname.split('/').at(-1))).toEqual([banks.id, telcos.id])
})

test('deleting a group says its customers are detached, then deletes it', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
    'DELETE /api/gw/admin/customer-groups/01a16026-0000-7000-8000-000000000001': () => empty(204),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click((await row('Banques')).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Supprimer le groupe Banques' })
  expect(dialog.textContent).toContain('Ses clients (3) sont détachés du groupe, pas supprimés.')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(calls.find((r) => r.method === 'DELETE')?.headers.get('Content-Type')).toBe('application/json')
  expect(calls.filter((r) => r.method === 'DELETE').map((r) => new URL(r.url).pathname)).toEqual([
    `/api/gw/admin/customer-groups/${banks.id}`,
  ])
  await waitFor(() =>
    expect(calls.filter((r) => r.method === 'GET' && r.url.includes('customer-groups')).length).toBe(2),
  )
})

test('without customer-groups:write every change is blocked and explained', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
  })
  renderAt('/groups')
  const reason = 'Cette action demande Groupes de clients : modifier customer-groups:write.'
  const create = await screen.findByRole('button', { name: 'Nouveau groupe' })
  expect(create.getAttribute('aria-disabled')).toBe('true')
  expect(explained(create)).toBe(reason)
  for (const name of ['Modifier', 'Archiver', 'Supprimer']) {
    const button = (await row('Banques')).getByRole('button', { name })
    expect(button.getAttribute('aria-disabled')).toBe('true')
    expect(explained(button)).toBe(reason)
  }
})

test('a gateway that does not answer offers to retry', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => json(503, { code: 'service_unavailable', message: 'down' }),
  })
  renderAt('/groups')
  expect(await screen.findByRole('heading', { name: 'Les groupes n’ont pas pu être chargés' })).toBeDefined()
  expect(screen.getByRole('button', { name: 'Réessayer' })).toBeDefined()
})

test('a group is created with the description typed', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () => page([]),
    'POST /api/gw/admin/customer-groups': () => json(201, banks),
  })
  const user = userEvent.setup()
  renderAt('/groups')
  await user.click(await screen.findByRole('button', { name: 'Nouveau groupe' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau groupe' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Banques')
  await user.type(within(dialog).getByLabelText('Description'), 'Établissements')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le groupe' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  const post = calls.find((r) => r.method === 'POST' && r.url.endsWith('/admin/customer-groups'))
  expect(await post?.json()).toEqual({ name: 'Banques', description: 'Établissements' })
})

test('a session gone while listing sends to the login', async () => {
  stubFetch({
    'GET /api/auth/me': once(() => json(200, reader)),
    'GET /api/gw/admin/customer-groups': () =>
      json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }),
  })
  const { router } = renderAt('/groups')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test('a refusal of the gateway is told as it comes', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () =>
      json(403, { code: 'forbidden_scope', message: 'the operator token lacks customer-groups:read' }),
  })
  renderAt('/groups')
  expect(await screen.findByText('the operator token lacks customer-groups:read')).toBeDefined()
})

test('a status the address does not know is dropped', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
  })
  renderAt('/groups?status=gone')
  await screen.findByRole('rowheader', { name: 'Banques' })
  const list = calls.find((r) => r.url.includes('customer-groups'))
  expect(new URL(list?.url ?? '').searchParams.has('status')).toBe(false)
})
