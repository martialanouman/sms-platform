import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, json, me, renderAt, stubFetch, violations } from '../test'

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
const page = (data: unknown[], next: string | null = null) => json(200, { data, next_cursor: next })

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
    'GET /api/gw/admin/customer-groups': () => page([banks, telcos]),
  })
  const { container } = renderAt('/groups')
  expect(await screen.findByRole('table', { name: 'Groupes de clients' })).toBeDefined()
  expect(document.title).toBe('Groupes de clients — Tableau de bord')
  expect((await cells('Banques')).slice(0, 3)).toEqual(['Établissements financiers', 'Actif', '3'])
  expect((await cells('Télécoms')).slice(0, 3)).toEqual(['—', 'Archivé', '0'])
  expect(await violations(container)).toEqual([])
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

  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Archivés' }))
  await waitFor(() => expect(router.state.location.search).toEqual({ status: 'archived' }))
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

  await screen.findByText('Banques')
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
  await waitFor(() => expect(calls.some((r) => r.method === 'DELETE')).toBe(true))
})

test('without customer-groups:write every change is blocked and explained', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customer-groups': () => page([banks]),
  })
  renderAt('/groups')
  expect((await screen.findByRole('button', { name: 'Nouveau groupe' })).getAttribute('aria-disabled')).toBe('true')
  for (const name of ['Modifier', 'Archiver', 'Supprimer']) {
    expect((await row('Banques')).getByRole('button', { name }).getAttribute('aria-disabled')).toBe('true')
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
