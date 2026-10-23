import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
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

function stub(who = reader) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    'GET /api/gw/admin/customer-groups': () => json(200, { data: [banks], next_cursor: null }),
    [`GET /api/gw/admin/customers/${acme.id}`]: () => json(200, { ...acme, group_id: null }),
    'GET /api/gw/admin/accounts': () =>
      json(200, {
        data: [account('api', 'active'), account('held', 'suspended'), account('old', 'closed')],
        next_cursor: null,
      }),
  })
}

const listed = (calls: Request[]) =>
  calls.filter((r) => new URL(r.url).pathname === '/api/gw/admin/accounts').map((r) => new URL(r.url).search)

test('the accounts list names their customer and their effective status', async () => {
  stub()
  const { container } = renderAt('/accounts')
  const table = await screen.findByRole('table', { name: 'Comptes' })
  const api = within(table).getByRole('row', { name: /api/ })
  expect(within(api).getByRole('link', { name: 'api' }).getAttribute('href')).toBe(`/accounts/${account('api', '').id}`)
  expect(within(api).getByRole('link', { name: 'Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`)
  expect(within(api).getByText('Actif (client suspendu)')).toBeDefined()
  expect(within(within(table).getByRole('row', { name: /old/ })).getByText('Clos')).toBeDefined()
  expect(within(within(table).getByRole('row', { name: /held/ })).getByText('Suspendu')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Nouveau compte' })).toBeNull()
  expect(await violations(container)).toEqual([])
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
  expect(await screen.findByText('Client : Acme')).toBeDefined()
  expect(listed(calls)[0]).toBe(`?customer_id=${acme.id}`)
  await user.click(screen.getByRole('button', { name: 'Retirer le filtre Client' }))
  await waitFor(() => expect(listed(calls).at(-1)).toBe(''))
  expect(screen.queryByText('Client : Acme')).toBeNull()
})

test('without customers:read the customer filter shows its identifier', async () => {
  const calls = stub({ ...me, permissions: ['accounts:read'] })
  renderAt(`/accounts?customer_id=${acme.id}`)
  await screen.findByRole('table', { name: 'Comptes' })
  expect(screen.getByText(`Client : ${acme.id}`)).toBeDefined()
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
