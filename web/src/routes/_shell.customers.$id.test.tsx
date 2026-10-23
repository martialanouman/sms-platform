import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { atMinute } from '../audit'
import { empty, explained, json, me, renderAt, stubFetch, toasted, violations } from '../test'

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
  const crumbs = screen.getByRole('navigation', { name: 'Fil d’Ariane' })
  expect(within(crumbs).getByRole('link', { name: 'Clients' }).getAttribute('href')).toBe('/customers')
  expect(within(crumbs).getByText('Acme').getAttribute('aria-current')).toBe('page')
  expect(within(crumbs).getByRole('link', { name: 'Clients' }).closest('li')?.hasAttribute('aria-current')).toBe(false)
  expect(screen.queryByRole('link', { name: '← Clients' })).toBeNull()
  await waitFor(() =>
    expect(screen.getByRole('link', { name: 'Banques' }).getAttribute('href')).toBe(`/customers?group_id=${banks.id}`),
  )
  const identity = screen.getByRole('region', { name: 'Identité' })
  expect(within(identity).getByText('Statut').nextElementSibling?.textContent).toBe('Actif')
  expect(within(identity).getByText('Groupe').nextElementSibling?.textContent).toBe('Banques')
  const ages = screen.getByRole('region', { name: 'Âges maximaux' })
  expect(within(ages).getByText('abaissent seulement le plafond de la plateforme')).toBeDefined()
  const age = (label: string) => within(ages).getByText(label).nextElementSibling as HTMLElement
  expect(age('OTP').textContent).toBe('60 s')
  expect(age('OTP').querySelector('.ui-mono')?.textContent).toBe('60 s')
  expect(age('Transactionnel').textContent).toBe(`Plafond de la plateforme ${(86400).toLocaleString('fr')} s`)
  expect(age('Transactionnel').querySelector('.ui-mono')?.textContent).toBe(`${(86400).toLocaleString('fr')} s`)
  expect(age('Marketing').textContent).toBe(`${(3600).toLocaleString('fr')} s`)
  const header = screen.getByRole('heading', { level: 1, name: 'Acme' }).closest('.page-header') as HTMLElement
  expect(within(header).getByText('Actif').className).toBe('ui-pill ui-pill--ok')
  expect(header.textContent).toContain(`Créé le ${atMinute(acme.created_at)} · modifié le ${atMinute(acme.updated_at)}`)
  expect(within(header).getByText(atMinute(acme.created_at)).className).toBe('ui-mono')
  expect(atMinute(acme.updated_at)).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/)
  expect(within(header).getByText(atMinute(acme.updated_at)).className).toBe('ui-mono')
  expect(
    within(header)
      .getAllByRole('button')
      .map((b) => [b.textContent, b.className.includes('ui-button--danger')]),
  ).toEqual([
    ['Modifier', false],
    ['Suspendre', false],
    ['Réactiver', false],
    ['Clôturer', true],
    ['Supprimer', true],
  ])
  expect(container.textContent).not.toContain(acme.id)
  expect(container.textContent).not.toMatch(/Comptes : \d/)
  expect(await violations(container)).toEqual([])
})

test('a customer without a group says so', async () => {
  stub({ ...acme, group_id: null })
  renderAt(`/customers/${acme.id}`)
  const identity = await screen.findByRole('region', { name: 'Identité' })
  expect(within(identity).getByText('Groupe').nextElementSibling?.textContent).toBe('Aucun')
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
    expect(dialog.textContent).toContain('2 comptes seront suspendus : api, bulk. 1 compte fermé le reste : old.'),
  )
  await confirm('Suspendre')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(calls.some((r) => r.method === 'POST' && r.url.endsWith('/suspend'))).toBe(true)
  expect(await toasted()).toEqual(['Client Acme suspendu.'])
  expect(await violations(document.body)).toEqual([])
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
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce client est fermé définitivement.')
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
  expect(await toasted()).toEqual(['Client Acme réactivé.'])
  await user.click(screen.getByRole('button', { name: 'Clôturer' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain('Définitif')
  await confirm('Clôturer')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await waitFor(async () => expect(await toasted()).toContain('Client Acme fermé.'))
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
      'Fermé définitivement.',
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
  expect(await toasted()).toEqual(['Client Acme supprimé.'])
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Clients' })))
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
  expect(await toasted()).toEqual(['Client Acme SA modifié.'])
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
  const crumbs = screen.getByRole('navigation', { name: 'Fil d’Ariane' })
  expect(within(crumbs).getByRole('link', { name: 'Clients' }).getAttribute('href')).toBe('/customers')
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
    expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce client est fermé définitivement.')
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
    'Cette action demande Clients : modifier customers:write.',
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
  expect(group.hasAttribute('disabled')).toBe(false)
  expect(group.getAttribute('tabindex')).not.toBe('-1')
  expect(group.getAttribute('aria-disabled')).toBe('true')
  await userEvent.setup().click(group)
  await expect(screen.findByRole('option', {}, { timeout: 300 })).rejects.toThrow()
  expect(document.getElementById(group.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Le groupe demande Groupes de clients : lire customer-groups:read.',
  )
  expect(calls.some((r) => r.url.includes('customer-groups'))).toBe(false)
})

const accountOf = (name: string, status: string) => ({
  id: `01a16026-0000-7000-8000-0000000a${name.padStart(4, '0')}`,
  customer_id: acme.id,
  customer_name: acme.name,
  customer_status: 'suspended',
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
const accountant = { ...me, permissions: [...writer.permissions, 'accounts:read', 'accounts:write'] }

test('the page of a customer lists its accounts, with their effective status, and links to them all', async () => {
  const calls = stub(
    { ...acme, status: 'suspended' },
    { 'GET /api/gw/admin/accounts': () => json(200, { data: [accountOf('api', 'active')], next_cursor: 'n' }) },
    accountant,
  )
  const { container } = renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  const row = await within(section).findByRole('row', { name: /api/ })
  expect(within(row).getByRole('link', { name: 'api' }).getAttribute('href')).toBe(
    `/accounts/${accountOf('api', '').id}`,
  )
  expect(within(row).getByRole('link', { name: 'api' }).className).toBe('ui-mono')
  expect(within(row).getByText('Suspendu').className).toBe('ui-pill ui-pill--warn')
  expect(within(row).getByText('par son client')).toBeDefined()
  expect(within(row).getByText('smpp · rest').className).toBe('ui-mono')
  expect(within(row).getByText(atMinute(accountOf('api', '').created_at)).className).toContain('ui-mono')
  expect(
    within(section)
      .getAllByRole('columnheader')
      .map((h) => h.textContent),
  ).toEqual(['Nom', 'Statut', 'Canaux', 'Créé le'])
  expect(within(section).getByRole('link', { name: 'Tous les comptes' }).getAttribute('href')).toBe(
    `/accounts?customer_id=${acme.id}`,
  )
  expect(within(section).getByRole('button', { name: 'Nouveau compte' }).className).toContain('ui-button--primary')
  expect(section.classList.contains('ui-card--flush')).toBe(true)
  expect(new URL(calls.find((r) => r.url.includes('/admin/accounts'))?.url ?? '').search).toBe(
    `?customer_id=${acme.id}`,
  )
  expect(await violations(container)).toEqual([])
})

test('a new account is created for the customer, and a taken name is said in French', async () => {
  let taken = false
  const calls = stub(
    acme,
    {
      'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }),
      'POST /api/gw/admin/accounts': () => {
        taken = !taken
        return taken
          ? json(409, { code: 'conflict', message: 'already taken', errors: [] })
          : json(201, accountOf('web', 'active'))
      },
    },
    accountant,
  )
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  await user.click(within(section).getByRole('button', { name: 'Nouveau compte' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau compte' })
  await user.type(within(dialog).getByRole('textbox', { name: 'Nom' }), 'web')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le compte' }))
  expect(await within(dialog).findByText('Un compte de ce client porte déjà ce nom.')).toBeDefined()
  await user.click(within(dialog).getByRole('button', { name: 'Créer le compte' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(
    await calls
      .filter((r) => r.method === 'POST')
      .at(-1)
      ?.json(),
  ).toEqual({ customer_id: acme.id, name: 'web' })
  expect(await toasted()).toEqual(['Compte web créé.'])
  await waitFor(() => expect(calls.filter((r) => r.url.includes('/admin/accounts?')).length).toBe(2))
})

test('a closed customer gets no new account', async () => {
  stub(
    { ...acme, status: 'closed' },
    { 'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }) },
    accountant,
  )
  renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  expect(explained(within(section).getByRole('button', { name: 'Nouveau compte' }))).toBe('Fermé définitivement.')
  expect(await within(section).findByText('Aucun compte.')).toBeDefined()
  expect(section.classList.contains('ui-card--flush')).toBe(false)
})

test('without accounts:read the accounts of a customer are not listed', async () => {
  const calls = stub(acme)
  renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  expect(section.textContent).toBe('ComptesLa liste des comptes demande Comptes : lire accounts:read.')
  expect(within(section).getByText('accounts:read').className).toBe('role-keys__key')
  expect(calls.some((r) => r.url.includes('/admin/accounts'))).toBe(false)
})

test('without accounts:write no account is created', async () => {
  stub(
    acme,
    { 'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }) },
    { ...me, permissions: [...writer.permissions, 'accounts:read'] },
  )
  renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  expect(explained(within(section).getByRole('button', { name: 'Nouveau compte' }))).toBe(
    'Cette action demande Comptes : modifier accounts:write.',
  )
})

test('suspending a customer reloads its accounts, which the cascade changed', async () => {
  const calls = stub(
    acme,
    {
      'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }),
      [`GET ${path}/suspension-impact`]: () =>
        json(200, { accounts_to_suspend: [], accounts_closed: [], binds_to_disconnect: [] }),
      [`POST ${path}/suspend`]: () => json(200, { ...acme, status: 'suspended' }),
    },
    accountant,
  )
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  await screen.findByText('Aucun compte.')
  await user.click(screen.getByRole('button', { name: 'Suspendre' }))
  await confirm('Suspendre')
  await waitFor(() => expect(calls.filter((r) => r.url.includes('/admin/accounts?')).length).toBe(2))
})

test('a new account refused because the customer is closed is said in French', async () => {
  stub(
    acme,
    {
      'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }),
      'POST /api/gw/admin/accounts': () => json(422, { code: 'validation_error', message: 'not allowed', errors: [] }),
    },
    accountant,
  )
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  const section = await screen.findByRole('region', { name: 'Comptes' })
  await user.click(within(section).getByRole('button', { name: 'Nouveau compte' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau compte' })
  await user.type(within(dialog).getByRole('textbox', { name: 'Nom' }), 'web')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le compte' }))
  expect(await within(dialog).findByText('Ce client est fermé ou n’existe plus.')).toBeDefined()
})

test('suspending by the keyboard leaves the focus on Suspendre, now blocked', async () => {
  let current = acme
  stub(acme, {
    [`GET ${path}`]: () => json(200, current),
    [`GET ${path}/suspension-impact`]: () =>
      json(200, { accounts_to_suspend: [], accounts_closed: [], binds_to_disconnect: [] }),
    [`POST ${path}/suspend`]: () => {
      current = { ...acme, status: 'suspended' }
      return json(200, current)
    },
  })
  const user = userEvent.setup()
  renderAt(`/customers/${acme.id}`)
  const suspend = await screen.findByRole('button', { name: 'Suspendre' })
  suspend.focus()
  await user.keyboard('{Enter}')
  const dialog = await screen.findByRole('alertdialog')
  within(dialog).getByRole('button', { name: 'Suspendre' }).focus()
  await user.keyboard('{Enter}')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await waitFor(() => expect(explained(screen.getByRole('button', { name: 'Suspendre' }))).toBe('Déjà suspendu.'))
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Suspendre' }))
})

test('without customer-groups:read the group of the customer is said unreadable', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['customers:read'] }),
    [`GET ${path}`]: () => json(200, acme),
  })
  const { container } = renderAt(`/customers/${acme.id}`)
  const identity = await screen.findByRole('region', { name: 'Identité' })
  expect(within(identity).getByText('Groupe').nextElementSibling?.textContent).toBe('non lisible')
  expect(container.textContent).not.toContain(banks.id)
  expect(await violations(container)).toEqual([])
})

test('without customers:read the breadcrumb names Clients without linking to it', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['accounts:read'] }) })
  renderAt(`/customers/${acme.id}`)
  const crumbs = await within(await screen.findByRole('banner')).findByRole('navigation', { name: 'Fil d’Ariane' })
  await waitFor(() => expect(crumbs.textContent).toBe('ClientsClient'))
  expect(within(crumbs).queryByRole('link')).toBeNull()
})
