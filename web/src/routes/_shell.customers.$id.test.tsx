import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, json, me, renderAt, stubFetch, violations } from '../test'

const writer = { ...me, permissions: ['customers:read', 'customers:write', 'customer-groups:read'] }
const reader = { ...me, permissions: ['customers:read', 'customer-groups:read'] }
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
const directory = () => json(200, { data: [banks], next_cursor: null })
const acme = {
  id: '01a16026-0000-7000-8000-0000000000c1',
  name: 'Acme',
  status: 'active',
  group_id: banks.id,
  max_age_override: { otp: 60, transactional: null, marketing: 3600 },
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T09:30:00Z',
}
const path = `/api/gw/admin/customers/${acme.id}`

function stub(customer: object, rest: Record<string, (r: Request) => Response | Promise<Response>> = {}, who = writer) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    'GET /api/gw/admin/customer-groups': directory,
    [`GET ${path}`]: () => json(200, customer),
    ...rest,
  })
}

async function confirm(action: string) {
  const dialog = await screen.findByRole('alertdialog')
  await userEvent.setup().click(within(dialog).getByRole('button', { name: action }))
  return dialog
}

test('the page of a customer shows who it is, its group and its ages', async () => {
  stub(acme)
  const { container } = renderAt(`/customers/${acme.id}`)
  expect(await screen.findByRole('heading', { name: 'Acme', level: 1 })).toBeDefined()
  expect(screen.getByRole('link', { name: '← Clients' }).getAttribute('href')).toBe('/customers')
  await waitFor(() =>
    expect(screen.getByRole('link', { name: 'Banques' }).getAttribute('href')).toBe(`/customers?group_id=${banks.id}`),
  )
  const identity = screen.getByRole('region', { name: 'Identité' })
  expect(within(identity).getByText('OTP').nextElementSibling?.textContent).toBe('60 s')
  expect(within(identity).getByText('Transactionnel').nextElementSibling?.textContent).toBe('Plafond de la plateforme')
  expect(within(identity).getByText('Marketing').nextElementSibling?.textContent).toBe('3600 s')
  expect(within(identity).getByText('Statut').nextElementSibling?.textContent).toBe('Actif')
  expect(await violations(container)).toEqual([])
})

test('suspending previews the accounts it suspends, then suspends', async () => {
  const calls = stub(acme, {
    [`GET ${path}/suspension-impact`]: () =>
      json(200, {
        accounts_to_suspend: [
          { id: 'a1', name: 'api' },
          { id: 'a2', name: 'bulk' },
        ],
        accounts_closed: [{ id: 'a3', name: 'old' }],
        binds_to_disconnect: [],
      }),
    [`POST ${path}/suspend`]: () => json(200, { ...acme, status: 'suspended' }),
  })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Suspendre le client Acme' })
  await waitFor(() =>
    expect(dialog.textContent).toContain('2 comptes seront suspendus : api, bulk. 1 compte clos le reste : old.'),
  )
  await confirm('Suspendre')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(calls.some((r) => r.method === 'POST' && r.url.endsWith('/suspend'))).toBe(true)
})

test('an impact that cannot be read does not prevent the suspension', async () => {
  const calls = stub(acme, {
    [`GET ${path}/suspension-impact`]: () => json(503, { code: 'service_unavailable', message: 'down' }),
    [`POST ${path}/suspend`]: () => json(200, { ...acme, status: 'suspended' }),
  })
  renderAt(`/customers/${acme.id}`)
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await screen.findByRole('alertdialog')
  await waitFor(() => expect(dialog.textContent).toContain('Impact indisponible.'))
  await confirm('Suspendre')
  await waitFor(() => expect(calls.some((r) => r.method === 'POST' && r.url.endsWith('/suspend'))).toBe(true))
})

test('a suspension refused because the customer is closed is said in French', async () => {
  stub(acme, {
    [`GET ${path}/suspension-impact`]: () =>
      json(200, { accounts_to_suspend: [], accounts_closed: [], binds_to_disconnect: [] }),
    [`POST ${path}/suspend`]: () => json(409, { code: 'conflict', message: 'not allowed' }),
  })
  renderAt(`/customers/${acme.id}`)
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await confirm('Suspendre')
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce client est clos définitivement.')
})

test('reactivating says its accounts stay suspended, and closing is definitive', async () => {
  const suspended = { ...acme, status: 'suspended' }
  const calls = stub(suspended, { [`PATCH ${path}`]: () => json(200, acme) })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Réactiver' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain(
    'Ses comptes restent suspendus ; réactivez-les un à un.',
  )
  await confirm('Réactiver')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await user.click(screen.getByRole('button', { name: 'Clôturer' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain('Définitif')
  await confirm('Clôturer')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  const patches = calls.filter((r) => r.method === 'PATCH')
  expect(await Promise.all(patches.map((r) => r.json()))).toEqual([{ status: 'active' }, { status: 'closed' }])
})

test('each status action is blocked and explained where it does nothing', async () => {
  stub({ ...acme, status: 'suspended' })
  renderAt(`/customers/${acme.id}`)
  const suspend = await screen.findByRole('button', { name: 'Suspendre' })
  expect(suspend.getAttribute('aria-disabled')).toBe('true')
  expect(document.getElementById(suspend.getAttribute('aria-describedby') ?? '')?.textContent).toBe('Déjà suspendu.')
  expect(screen.getByRole('button', { name: 'Réactiver' }).hasAttribute('aria-disabled')).toBe(false)
})

test('an active customer cannot be reactivated', async () => {
  stub(acme)
  renderAt(`/customers/${acme.id}`)
  const reactivate = await screen.findByRole('button', { name: 'Réactiver' })
  expect(document.getElementById(reactivate.getAttribute('aria-describedby') ?? '')?.textContent).toBe('Déjà actif.')
})

test('a closed customer is frozen but can still be deleted', async () => {
  stub({ ...acme, status: 'closed' })
  renderAt(`/customers/${acme.id}`)
  for (const name of ['Suspendre', 'Réactiver', 'Clôturer', 'Modifier']) {
    const button = await screen.findByRole('button', { name })
    expect(document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
      'Clos définitivement.',
    )
  }
  expect(screen.getByRole('button', { name: 'Supprimer' }).hasAttribute('aria-disabled')).toBe(false)
})

test('a customer with accounts is not deleted, and the reason is in French', async () => {
  stub(acme, { [`DELETE ${path}`]: () => json(409, { code: 'conflict', message: 'still referenced' }) })
  renderAt(`/customers/${acme.id}`)
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Supprimer' }))
  const dialog = await confirm('Supprimer')
  expect((await within(dialog).findByRole('alert')).textContent).toBe(
    'Ce client a des comptes : suspendez-le ou clôturez-le.',
  )
})

test('a deleted customer leads back to the list', async () => {
  const calls = stub(acme, {
    [`DELETE ${path}`]: () => empty(204),
    'GET /api/gw/admin/customers': () => json(200, { data: [], next_cursor: null }),
  })
  const { router } = renderAt(`/customers/${acme.id}`)
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Supprimer' }))
  await confirm('Supprimer')
  await waitFor(() => expect(router.state.location.pathname).toBe('/customers'))
  expect(calls.filter((r) => r.method === 'DELETE').map((r) => new URL(r.url).pathname)).toEqual([path])
})

test('an edit sends what changed: the group cleared, and the three ages together', async () => {
  const calls = stub(acme, { [`PATCH ${path}`]: () => json(200, acme) })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le client Acme' })
  expect((within(dialog).getByLabelText('OTP (s)') as HTMLInputElement).value).toBe('60')
  await user.click(within(dialog).getByRole('combobox', { name: 'Groupe' }))
  await user.click(await screen.findByRole('option', { name: 'Aucun groupe' }))
  await user.clear(within(dialog).getByLabelText('OTP (s)'))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({
    group_id: null,
    max_age_override: { otp: null, transactional: null, marketing: 3600 },
  })
})

test('an edit of the name alone sends the name alone', async () => {
  const calls = stub(acme, { [`PATCH ${path}`]: () => json(200, acme) })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le client Acme' })
  await user.type(within(dialog).getByLabelText('Nom'), ' SA')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({ name: 'Acme SA' })
})

test('without customers:write every change is blocked', async () => {
  stub(acme, {}, reader)
  renderAt(`/customers/${acme.id}`)
  for (const name of ['Modifier', 'Suspendre', 'Clôturer', 'Supprimer']) {
    expect((await screen.findByRole('button', { name })).getAttribute('aria-disabled')).toBe('true')
  }
})

test('an unknown customer is told', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': directory,
    [`GET ${path}`]: () => json(404, { code: 'not_found', message: 'not found' }),
  })
  renderAt(`/customers/${acme.id}`)
  expect(await screen.findByRole('heading', { name: 'Ce client n’a pas pu être chargé' })).toBeDefined()
})

test('reactivating or closing a customer closed meanwhile is said in French', async () => {
  for (const [start, action] of [
    [{ ...acme, status: 'suspended' }, 'Réactiver'],
    [acme, 'Clôturer'],
  ] as const) {
    stub(start, { [`PATCH ${path}`]: () => json(422, { code: 'validation_error', message: 'not allowed' }) })
    const { unmount } = renderAt(`/customers/${acme.id}`)
    await userEvent.setup().click(await screen.findByRole('button', { name: action }))
    const dialog = await confirm(action)
    expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce client est clos définitivement.')
    unmount()
  }
})

test('an edit refused because the group is gone is said in French', async () => {
  stub(acme, {
    [`PATCH ${path}`]: () => json(422, { code: 'validation_error', message: 'unknown reference' }),
  })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le client Acme' })
  await user.type(within(dialog).getByLabelText('Nom'), ' SA')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce groupe n’existe plus.')
})

test('the archived group a customer is in is still offered when editing it', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/gw/admin/customer-groups': () =>
      json(200, { data: [{ ...banks, status: 'archived' }], next_cursor: null }),
    [`GET ${path}`]: () => json(200, acme),
  })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le client Acme' })
  await waitFor(() =>
    expect(within(dialog).getByRole('combobox', { name: 'Groupe' }).textContent).toBe('Groupe : Banques (archivé)'),
  )
})

test('a suspension that suspends no account says so', async () => {
  stub(acme, {
    [`GET ${path}/suspension-impact`]: () =>
      json(200, { accounts_to_suspend: [], accounts_closed: [], binds_to_disconnect: [] }),
  })
  renderAt(`/customers/${acme.id}`)
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await screen.findByRole('alertdialog')
  await waitFor(() => expect(dialog.textContent).toContain('Aucun compte à suspendre.'))
})

test('a reader cannot reactivate a suspended customer', async () => {
  stub({ ...acme, status: 'suspended' }, {}, reader)
  renderAt(`/customers/${acme.id}`)
  const reactivate = await screen.findByRole('button', { name: 'Réactiver' })
  expect(document.getElementById(reactivate.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Cette action demande customers:write.',
  )
})

test('without customer-groups:read the group of the form is blocked and explained', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['customers:read', 'customers:write'] }),
    [`GET ${path}`]: () => json(200, acme),
  })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await user.click(await screen.findByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le client Acme' })
  const group = within(dialog).getByRole('combobox', { name: 'Groupe' })
  expect(group.hasAttribute('disabled') || group.getAttribute('aria-disabled') === 'true').toBe(true)
  expect(document.getElementById(group.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Le groupe demande customer-groups:read.',
  )
  expect(calls.some((r) => r.url.includes('customer-groups'))).toBe(false)
})
