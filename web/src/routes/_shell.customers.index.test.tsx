import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { atMinute } from '../audit'
import { explained, json, me, once, renderAt, stubFetch, toasted, violations } from '../test'

const reader = { ...me, permissions: ['customers:read', 'customer-groups:read'] }
const writer = { ...reader, permissions: [...reader.permissions, 'customers:write'] }
const banks = { id: '01a16026-0000-7000-8000-0000000000b1', name: 'Banques', status: 'active' }
const old = { id: '01a16026-0000-7000-8000-0000000000b2', name: 'Anciens', status: 'archived' }
const telcos = { id: '01a16026-0000-7000-8000-0000000000b3', name: 'Télécoms', status: 'active' }
const group = (g: typeof banks) => ({
  ...g,
  description: null,
  member_count: 1,
  created_by: null,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
})
const directory = () => json(200, { data: [group(telcos), group(old), group(banks)], next_cursor: null })
const acme = {
  id: '01a16026-0000-7000-8000-0000000000c1',
  name: 'Acme',
  status: 'active',
  group_id: banks.id,
  max_age_override: { otp: null, transactional: null, marketing: null },
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
}
const solo = { ...acme, id: '01a16026-0000-7000-8000-0000000000c2', name: 'Solo', status: 'suspended', group_id: null }
const page = (data: unknown[], next: string | null = null) => json(200, { data, next_cursor: next })

async function cells(name: string) {
  const row = (await screen.findByRole('rowheader', { name })).closest('tr') as HTMLTableRowElement
  return within(row)
    .getAllByRole('cell')
    .map((c) => c.textContent)
}

test('the rail offers Clients before Groupes', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  const clients = await screen.findByRole('list', { name: 'Clients' })
  expect(
    within(clients)
      .getAllByRole('link')
      .map((l) => l.textContent),
  ).toEqual(['Clients', 'Groupes'])
})

test('without customers:read the screen says why and asks nothing', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['audit:read'] }) })
  renderAt('/customers')
  expect(await screen.findByRole('heading', { name: 'Accès aux clients non accordé' })).toBeDefined()
  expect(calls.some((r) => new URL(r.url).pathname.startsWith('/api/gw/'))).toBe(false)
})

test('the customers are listed with their status in a pill and their group named, and lead to their page', async () => {
  const gone = { ...solo, id: '01a16026-0000-7000-8000-0000000000c3', name: 'Gone', status: 'closed', group_id: old.id }
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme, solo, gone]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  const { container } = renderAt('/customers')
  expect(await screen.findByRole('table', { name: 'Clients' })).toBeDefined()
  expect(document.title).toBe('Clients — Tableau de bord')
  expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Nom', 'Statut', 'Groupe', 'Créé le'])
  await waitFor(async () => expect(await cells('Acme')).toEqual(['Actif', 'Banques', atMinute(acme.created_at)]))
  expect(atMinute(acme.created_at)).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/)
  expect(await cells('Solo')).toEqual(['Suspendu', 'Aucun', atMinute(solo.created_at)])
  expect(await cells('Gone')).toEqual(['Fermé', 'Anciens (archivé)', atMinute(gone.created_at)])
  const pill = async (name: string, label: string) =>
    within((await screen.findByRole('rowheader', { name })).closest('tr') as HTMLElement).getByText(label).className
  expect(await pill('Acme', 'Actif')).toBe('ui-pill ui-pill--ok')
  expect(await pill('Solo', 'Suspendu')).toBe('ui-pill ui-pill--warn')
  expect(await pill('Gone', 'Fermé')).toBe('ui-pill ui-pill--neutral')
  expect(await pill('Acme', atMinute(acme.created_at))).toContain('ui-mono')
  expect(screen.getByRole('link', { name: 'Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`)
  const groups = calls.find((r) => r.url.includes('customer-groups'))
  expect(new URL(groups?.url ?? '').searchParams.get('limit')).toBe('500')
  expect(new URL(groups?.url ?? '').searchParams.has('status')).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('the header says what a customer is and offers to create one; the table and its pager share a card', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => page([acme]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  const table = await screen.findByRole('table', { name: 'Clients' })
  const header = screen.getByRole('heading', { level: 1, name: 'Clients' }).closest('.page-header') as HTMLElement
  expect(header.textContent).toContain(
    'Les clients de l’agrégateur, leur groupe et leur statut. Un client porte un ou plusieurs comptes.',
  )
  expect(within(header).getByRole('button', { name: 'Nouveau client' })).toBeDefined()
  const card = table.closest('.ui-card') as HTMLElement
  expect(card.classList.contains('ui-card--flush')).toBe(true)
  expect(within(card).getByText('1 client affiché')).toBeDefined()
})

test('no customer at all says how to create one', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => page([]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  const { container } = renderAt('/customers')
  expect(await screen.findByText('Aucun client')).toBeDefined()
  expect(screen.getByText('Créez le premier client avec « Nouveau client ».')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Retirer les filtres' })).toBeNull()
  expect(await violations(container)).toEqual([])
})

test('no customer under either filter asks to widen them, and clears them', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => page([]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  for (const search of ['status=closed', `group_id=${banks.id}`]) {
    const { router, unmount } = renderAt(`/customers?${search}`)
    expect(await screen.findByText('Aucun résultat')).toBeDefined()
    expect(screen.getByText('Élargissez les filtres.')).toBeDefined()
    expect(screen.queryByText('Aucun client')).toBeNull()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Retirer les filtres' }))
    await waitFor(() => expect(router.state.location.search).toEqual({}))
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Clients' })))
    unmount()
  }
})

test('the group and status filters travel in the address', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  const user = userEvent.setup()
  const { router } = renderAt('/customers')
  await screen.findByRole('rowheader', { name: 'Acme' })

  const groupFilter = screen.getByRole('combobox', { name: 'Groupe' })
  expect(groupFilter.textContent).toBe('Groupe : Tous')
  await user.click(groupFilter)
  const options = await screen.findAllByRole('option')
  expect(options.map((o) => o.textContent)).toEqual(['Tous', 'Anciens (archivé)', 'Banques', 'Télécoms'])
  await user.click(screen.getByRole('option', { name: 'Banques' }))
  await waitFor(() => expect(router.state.location.search).toEqual({ group_id: banks.id }))

  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual([
    'Tous',
    'Actifs',
    'Suspendus',
    'Fermés',
  ])
  await user.click(await screen.findByRole('option', { name: 'Suspendus' }))
  await waitFor(() => expect(router.state.location.search).toEqual({ group_id: banks.id, status: 'suspended' }))
  await waitFor(() => {
    const last = new URL(calls.filter((r) => r.url.includes('/admin/customers')).at(-1)?.url ?? '')
    expect([last.searchParams.get('group_id'), last.searchParams.get('status')]).toEqual([banks.id, 'suspended'])
  })
})

test('a customer is created in an active group with the ages typed', async () => {
  let listed = 0
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => (listed++ === 0 ? page([]) : page([acme])),
    'GET /api/gw/admin/customer-groups': directory,
    'POST /api/gw/admin/customers': () => json(201, acme),
  })
  const user = userEvent.setup()
  renderAt('/customers')
  await user.click(await screen.findByRole('button', { name: 'Nouveau client' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau client' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Acme')
  await user.click(within(dialog).getByRole('combobox', { name: 'Groupe' }))
  const options = await screen.findAllByRole('option')
  expect(options.map((o) => o.textContent)).toEqual(['Aucun groupe', 'Banques', 'Télécoms'])
  await user.click(screen.getByRole('option', { name: 'Banques' }))
  await user.type(within(dialog).getByLabelText('OTP (s)'), '60')
  expect(within(dialog).getByLabelText('Marketing (s)').getAttribute('placeholder')).toBe('259200')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le client' }))

  await screen.findByRole('rowheader', { name: 'Acme' })
  const post = calls.find((r) => r.method === 'POST' && r.url.endsWith('/admin/customers'))
  expect(await post?.json()).toEqual({
    name: 'Acme',
    group_id: banks.id,
    max_age_override: { otp: 60, transactional: null, marketing: null },
  })
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(await toasted()).toEqual(['Client Acme créé.'])
})

test('a bare customer is created with its name alone', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => page([]),
    'GET /api/gw/admin/customer-groups': directory,
    'POST /api/gw/admin/customers': () => json(201, solo),
  })
  const user = userEvent.setup()
  renderAt('/customers')
  await user.click(await screen.findByRole('button', { name: 'Nouveau client' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau client' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Solo')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le client' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  const post = calls.find((r) => r.method === 'POST' && r.url.endsWith('/admin/customers'))
  expect(await post?.json()).toEqual({ name: 'Solo' })
})

test('a group gone meanwhile is said in French', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customers': () => page([]),
    'GET /api/gw/admin/customer-groups': directory,
    'POST /api/gw/admin/customers': () =>
      json(422, {
        code: 'validation_error',
        message: 'unknown reference',
        errors: [{ field: 'body.group_id', message: 'x' }],
      }),
  })
  const user = userEvent.setup()
  renderAt('/customers')
  await user.click(await screen.findByRole('button', { name: 'Nouveau client' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau client' })
  await user.type(within(dialog).getByLabelText('Nom'), 'Acme')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le client' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce groupe n’existe plus.')
})

test('without customers:write the creation is blocked and explained', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  expect((await screen.findByRole('button', { name: 'Nouveau client' })).getAttribute('aria-disabled')).toBe('true')
})

test('a session gone while listing sends to the login', async () => {
  stubFetch({
    'GET /api/auth/me': once(() => json(200, reader)),
    'GET /api/gw/admin/customers': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }),
    'GET /api/gw/admin/customer-groups': directory,
  })
  const { router } = renderAt('/customers')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test('the customers that cannot be read are told, with a retry', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => json(503, { code: 'service_unavailable', message: 'down' }),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  expect(await screen.findByRole('heading', { name: 'Les clients n’ont pas pu être chargés' })).toBeDefined()
})

test('a group given in the address filters the list from the start', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt(`/customers?group_id=${banks.id}`)
  await screen.findByRole('rowheader', { name: 'Acme' })
  const list = calls.find((r) => r.url.includes('/admin/customers'))
  expect(new URL(list?.url ?? '').searchParams.get('group_id')).toBe(banks.id)
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Groupe' }).textContent).toBe('Groupe : Banques'))
})

test('the next page is asked with its cursor', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': (r) =>
      new URL(r.url).searchParams.get('cursor') ? page([solo]) : page([acme], 'c1'),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Suivant' }))
  await screen.findByRole('rowheader', { name: 'Solo' })
  const last = new URL(calls.filter((r) => r.url.includes('/admin/customers')).at(-1)?.url ?? '')
  expect(last.searchParams.get('cursor')).toBe('c1')
})

test('without customer-groups:read the group filter is blocked and explained', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['customers:read'] }),
    'GET /api/gw/admin/customers': () => page([acme]),
  })
  renderAt(`/customers?group_id=${banks.id}`)
  const group = await screen.findByRole('combobox', { name: 'Groupe' })
  expect(group.textContent).toBe(`Groupe : ${banks.id}`)
  expect(group.hasAttribute('disabled')).toBe(false)
  expect(group.getAttribute('tabindex')).not.toBe('-1')
  expect(group.getAttribute('aria-disabled')).toBe('true')
  await userEvent.setup().click(group)
  await expect(screen.findByRole('option', {}, { timeout: 300 })).rejects.toThrow()
  expect(document.getElementById(group.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Le groupe demande Groupes de clients : lire customer-groups:read.',
  )
})

test('the pager tells how many are shown, its useless buttons frozen and explained', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme, solo]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  expect(await screen.findByText('2 clients affichés')).toBeDefined()
  const previous = screen.getByRole('button', { name: 'Précédent' })
  const next = screen.getByRole('button', { name: 'Suivant' })
  expect([previous, next].map((b) => b.getAttribute('aria-disabled'))).toEqual(['true', 'true'])
  expect([explained(previous), explained(next)]).toEqual(['Première page.', 'Dernière page.'])
})

test('a single customer shown is said in the singular, with a way forward', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme], 'c1'),
    'GET /api/gw/admin/customer-groups': directory,
  })
  renderAt('/customers')
  expect(await screen.findByText('1 client affiché')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Suivant' }).hasAttribute('aria-disabled')).toBe(false)
})

test('an active status from the address filters, and Tous on the group leaves the address bare', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, reader),
    'GET /api/gw/admin/customers': () => page([acme]),
    'GET /api/gw/admin/customer-groups': directory,
  })
  const user = userEvent.setup()
  const { router } = renderAt(`/customers?status=active&group_id=${banks.id}`)
  await screen.findByRole('rowheader', { name: 'Acme' })
  expect(new URL(calls.find((r) => r.url.includes('/admin/customers'))?.url ?? '').searchParams.get('status')).toBe(
    'active',
  )
  expect(screen.getByRole('combobox', { name: 'Statut' }).textContent).toBe('Statut : Actifs')
  await user.click(screen.getByRole('combobox', { name: 'Groupe' }))
  await user.click(await screen.findByRole('option', { name: 'Tous' }))
  await waitFor(() => expect(router.state.location.href).toBe('/customers?status=active'))
})

test('without customer-groups:read the group of a customer is said unreadable, never its identifier', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['customers:read'] }),
    'GET /api/gw/admin/customers': () => page([acme, solo]),
  })
  const { container } = renderAt('/customers')
  const table = await screen.findByRole('table', { name: 'Clients' })
  expect((await cells('Acme'))[1]).toBe('non lisible')
  expect((await cells('Solo'))[1]).toBe('Aucun')
  expect(table.textContent).not.toContain(banks.id)
  expect(await violations(container)).toEqual([])
})
