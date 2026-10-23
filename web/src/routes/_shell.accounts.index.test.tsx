import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { atMinute } from '../audit'
import { json, me, renderAt, stubFetch, violations } from '../test'

const reader = { ...me, permissions: ['accounts:read', 'customers:read', 'customer-groups:read'] }
const banks = {
  id: '01a16026-0000-7000-8000-0000000000b1',
  name: 'Banques',
  description: null,
  status: 'active',
  member_count: 1,
  created_by: null,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
}
const acme = { id: '01a16026-0000-7000-8000-0000000000c1', name: 'Acme', status: 'suspended' }
const account = (name: string, status: string, customer = acme) => ({
  id: `01a16026-0000-7000-8000-0000000a${name.padStart(4, '0')}`,
  customer_id: customer.id,
  customer_name: customer.name,
  customer_status: customer.status,
  name,
  status,
  smpp_enabled: true,
  rest_enabled: true,
  query_sm_enabled: true,
  cancel_sm_enabled: true,
  allowed_bind_types: ['transceiver'],
  max_sessions: 1,
  rate_limit: null,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
})

const three = [
  account('api', 'active'),
  { ...account('held', 'suspended'), smpp_enabled: false },
  { ...account('old', 'closed'), rest_enabled: false },
]

function stub(who = reader, accounts: unknown[] = three) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    'GET /api/gw/admin/customer-groups': () => json(200, { data: [banks], next_cursor: null }),
    [`GET /api/gw/admin/customers/${acme.id}`]: () => json(200, { ...acme, group_id: null }),
    'GET /api/gw/admin/accounts': () => json(200, { data: accounts, next_cursor: null }),
  })
}

const listed = (calls: Request[]) =>
  calls.filter((r) => new URL(r.url).pathname === '/api/gw/admin/accounts').map((r) => new URL(r.url).search)

test('the accounts list names their customer, their effective status in a pill and their channels', async () => {
  stub()
  const { container } = renderAt('/accounts')
  const table = await screen.findByRole('table', { name: 'Comptes' })
  expect(
    within(table)
      .getAllByRole('columnheader')
      .map((h) => h.textContent),
  ).toEqual(['Nom', 'Client', 'Statut', 'Canaux', 'Créé le'])
  const api = within(table).getByRole('row', { name: /api/ })
  const name = within(api).getByRole('link', { name: 'api' })
  expect(name.getAttribute('href')).toBe(`/accounts/${account('api', '').id}`)
  expect(name.className).toBe('ui-mono')
  expect(within(api).getByRole('link', { name: 'Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`)
  expect(within(api).getByText('Suspendu').className).toBe('ui-pill ui-pill--warn')
  expect(within(api).getByText('par son client')).toBeDefined()
  expect(within(api).getByText('smpp · rest').className).toBe('ui-mono')
  const held = within(table).getByRole('row', { name: /held/ })
  expect(within(held).getByText('Suspendu')).toBeDefined()
  expect(within(held).queryByText('par son client')).toBeNull()
  expect(within(held).getByText('rest')).toBeDefined()
  const old = within(table).getByRole('row', { name: /old/ })
  expect(within(old).getByText('Fermé').className).toBe('ui-pill ui-pill--neutral')
  expect(within(old).getByText('smpp')).toBeDefined()
  expect(atMinute(account('old', '').created_at)).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/)
  expect(within(old).getByText(atMinute(account('old', '').created_at)).className).toContain('ui-mono')
  expect(screen.getByText('3 comptes affichés').closest('.ui-card')).toBe(table.closest('.ui-card'))
  expect(screen.queryByRole('button', { name: 'Nouveau compte' })).toBeNull()
  expect(
    (screen.getByRole('heading', { level: 1, name: 'Comptes' }).closest('.page-header') as HTMLElement).textContent,
  ).toContain('Les comptes SMPP et REST de tous les clients. Un compte se crée depuis la fiche de son client.')
  expect(await violations(container)).toEqual([])
})

test('no account at all says where one is created', async () => {
  stub(reader, [])
  renderAt('/accounts')
  expect(await screen.findByText('Aucun compte')).toBeDefined()
  expect(screen.getByText('Un compte se crée depuis la fiche de son client.')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Retirer les filtres' })).toBeNull()
})

test('no account under any filter asks to widen them, and clears them all', async () => {
  stub(reader, [])
  for (const search of [`customer_id=${acme.id}`, 'status=closed', `group_id=${banks.id}`]) {
    const { router, container, unmount } = renderAt(`/accounts?${search}`)
    expect(await screen.findByText('Aucun résultat')).toBeDefined()
    expect(screen.getByText('Élargissez les filtres.')).toBeDefined()
    expect(await violations(container)).toEqual([])
    await userEvent.setup().click(screen.getByRole('button', { name: 'Retirer les filtres' }))
    await waitFor(() => expect(router.state.location.search).toEqual({}))
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Comptes' })))
    unmount()
  }
})

test('the accounts are filtered by group and status through the address', async () => {
  const calls = stub()
  const user = userEvent.setup()
  renderAt('/accounts')
  await screen.findByRole('table', { name: 'Comptes' })
  await user.click(screen.getByRole('combobox', { name: 'Groupe' }))
  await user.click(await screen.findByRole('option', { name: 'Banques' }))
  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Suspendus' }))
  await waitFor(() => expect(listed(calls).at(-1)).toBe(`?group_id=${banks.id}&status=suspended`))
})

test('a customer filter from the address is shown with its name and can be removed', async () => {
  const calls = stub()
  const user = userEvent.setup()
  renderAt(`/accounts?customer_id=${acme.id}`)
  const chip = await screen.findByText('Client : Acme')
  expect(chip.className).toBe('ui-chip')
  expect(listed(calls)[0]).toBe(`?customer_id=${acme.id}`)
  const remove = within(chip).getByRole('button', { name: 'Retirer le filtre Client' })
  expect(remove.textContent).toBe('')
  await user.click(remove)
  await waitFor(() => expect(listed(calls).at(-1)).toBe(''))
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Comptes' })))
  expect(screen.queryByText('Client : Acme')).toBeNull()
})

test('without customers:read the customer filter is named after its listed accounts', async () => {
  const calls = stub({ ...me, permissions: ['accounts:read'] })
  renderAt(`/accounts?customer_id=${acme.id}`)
  await screen.findByRole('table', { name: 'Comptes' })
  expect(screen.getByText('Client : Acme')).toBeDefined()
  expect(document.body.textContent).not.toContain(acme.id)
  expect(calls.some((r) => r.url.includes('/admin/customers/'))).toBe(false)
})

test('without accounts:read the list is refused', async () => {
  stub({ ...me, permissions: ['customers:read'] })
  renderAt('/accounts')
  expect(await screen.findByText('Accès aux comptes non accordé')).toBeDefined()
})

test('the rail offers Comptes between Clients and Groupes', async () => {
  stub()
  renderAt('/accounts')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  const clients = await within(rail).findByRole('list', { name: 'Clients' })
  expect(
    within(clients)
      .getAllByRole('link')
      .map((l) => l.textContent),
  ).toEqual(['Clients', 'Comptes', 'Groupes'])
})

test('choosing a filter keeps the customer and drops the cursor', async () => {
  const calls = stub()
  const user = userEvent.setup()
  renderAt(`/accounts?customer_id=${acme.id}&cursor=c1`)
  await screen.findByRole('table', { name: 'Comptes' })
  await user.click(screen.getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Suspendus' }))
  await waitFor(() => expect(listed(calls).at(-1)).toBe(`?customer_id=${acme.id}&status=suspended`))
})
