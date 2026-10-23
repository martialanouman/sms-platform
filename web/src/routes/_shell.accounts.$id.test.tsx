import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, explained, json, me, renderAt, stubFetch, violations } from '../test'

const writer = { ...me, permissions: ['accounts:read', 'accounts:write'] }
const acme = { id: '01a16026-0000-7000-8000-0000000000c1' }
const api = {
  id: '01a16026-0000-7000-8000-0000000000a1',
  customer_id: acme.id,
  customer_name: 'Acme',
  customer_status: 'active',
  name: 'api',
  status: 'active',
  smpp_enabled: true,
  rest_enabled: false,
  query_sm_enabled: true,
  cancel_sm_enabled: false,
  allowed_bind_types: ['transceiver'],
  max_sessions: 2,
  rate_limit: null,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T09:30:00Z',
}
const path = `/api/gw/admin/accounts/${api.id}`

function stub(account: object, rest: Record<string, (r: Request) => Response | Promise<Response>> = {}, who = writer) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET ${path}`]: () => json(200, account),
    ...rest,
  })
}

const sent = async (calls: Request[], method: string, suffix = '') =>
  await calls.find((r) => r.method === method && new URL(r.url).pathname === path + suffix)?.json()

async function editBlock(name: string, render = true) {
  const user = userEvent.setup()
  if (render) {
    renderAt(`/accounts/${api.id}`)
  }
  const block = await screen.findByRole('region', { name })
  await user.click(within(block).getByRole('button', { name: 'Modifier' }))
  return { user, dialog: await screen.findByRole('dialog') }
}

test('the page of an account shows its four blocks and leads back to its customer', async () => {
  stub(api)
  const { container } = renderAt(`/accounts/${api.id}`)
  expect(await screen.findByRole('heading', { name: 'api', level: 1 })).toBeDefined()
  await waitFor(() =>
    expect(screen.getByRole('link', { name: '← Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`),
  )
  const value = (block: string, term: string) =>
    within(screen.getByRole('region', { name: block })).getByText(term).nextElementSibling?.textContent
  expect(value('Identité', 'Statut')).toBe('Actif')
  expect(value('Canaux', 'SMPP')).toBe('Activé')
  expect(value('Canaux', 'REST')).toBe('Désactivé')
  expect(value('Sessions', 'Sessions simultanées')).toBe('2')
  expect(value('Sessions', 'Types de bind')).toBe('transceiver')
  expect(value('Opérations SMPP', 'query_sm')).toBe('Activé')
  expect(value('Opérations SMPP', 'cancel_sm')).toBe('Désactivé')
  expect(await violations(container)).toEqual([])
})

test('an account under a suspended customer is shown suspended in effect', async () => {
  stub({ ...api, customer_status: 'suspended' })
  renderAt(`/accounts/${api.id}`)
  const identity = await screen.findByRole('region', { name: 'Identité' })
  expect(within(identity).getByText('Statut').nextElementSibling?.textContent).toBe('Actif (client suspendu)')
})

test('the channels are sent together, and the last one enabled cannot be disabled', async () => {
  const calls = stub(api, { [`PATCH ${path}/channels`]: () => json(200, { ...api, rest_enabled: true }) })
  const { user, dialog } = await editBlock('Canaux')
  const smpp = within(dialog).getByRole('checkbox', { name: 'SMPP' })
  expect(smpp.getAttribute('aria-disabled') ?? smpp.getAttribute('data-disabled')).not.toBeNull()
  expect(within(dialog).getByText('Au moins un canal reste activé.')).toBeDefined()
  await user.click(within(dialog).getByRole('checkbox', { name: 'REST' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/channels')).toEqual({ smpp_enabled: true, rest_enabled: true })
})

test('the session limits are sent with the bind types in a fixed order', async () => {
  const calls = stub(api, { [`PATCH ${path}/session-limits`]: () => json(200, api) })
  const { user, dialog } = await editBlock('Sessions')
  const sessions = within(dialog).getByRole('spinbutton', { name: 'Sessions simultanées' })
  expect(sessions.getAttribute('max')).toBe('2147483647')
  await user.clear(sessions)
  await user.type(sessions, '0')
  await user.click(within(dialog).getByRole('checkbox', { name: 'receiver' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'transmitter' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/session-limits')).toEqual({
    max_sessions: 0,
    allowed_bind_types: ['transmitter', 'receiver', 'transceiver'],
  })
})

test('the SMPP operations send only what changed, and nothing when nothing did', async () => {
  const calls = stub(api, { [`PATCH ${path}/smpp-ops`]: () => json(200, api) })
  let { user, dialog } = await editBlock('Opérations SMPP')
  expect(dialog.textContent).toContain('Les drapeaux se lisent au bind : les binds ouverts du compte sont déconnectés.')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(calls.some((r) => r.method === 'PATCH')).toBe(false)
  ;({ user, dialog } = await editBlock('Opérations SMPP', false))
  await user.click(within(dialog).getByRole('checkbox', { name: 'cancel_sm' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/smpp-ops')).toEqual({ cancel_sm_enabled: true })
})

test('a rename onto a taken name is refused in French', async () => {
  const calls = stub(api, {
    [`PATCH ${path}`]: () => json(409, { code: 'conflict', message: 'already taken', errors: [] }),
  })
  const { user, dialog } = await editBlock('Identité')
  const name = within(dialog).getByRole('textbox', { name: 'Nom' })
  await user.clear(name)
  await user.type(name, 'web')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect(await within(dialog).findByText('Un compte de ce client porte déjà ce nom.')).toBeDefined()
  expect(await sent(calls, 'PATCH')).toEqual({ name: 'web' })
})

test('suspending, reactivating under a suspended customer, and closing', async () => {
  const calls = stub(
    { ...api, status: 'suspended', customer_status: 'suspended' },
    {
      [`PATCH ${path}`]: () => json(200, api),
    },
  )
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(await screen.findByRole('button', { name: 'Réactiver' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Réactiver le compte api' })
  expect(dialog.textContent).toContain('Son client est suspendu : le compte reste suspendu en pratique.')
  await user.click(within(dialog).getByRole('button', { name: 'Réactiver' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(explained(screen.getByRole('button', { name: 'Suspendre' }))).toBe('Déjà suspendu.')
  await user.click(screen.getByRole('button', { name: 'Clôturer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Clôturer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  const patches = calls.filter((r) => r.method === 'PATCH')
  expect(await Promise.all(patches.map((r) => r.json()))).toEqual([{ status: 'active' }, { status: 'closed' }])
})

test('a suspension refused because the account is closed is said in French', async () => {
  stub(api, { [`POST ${path}/suspend`]: () => json(409, { code: 'conflict', message: 'closed', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain('Ses binds ouverts sont déconnectés.')
  await user.click(within(dialog).getByRole('button', { name: 'Suspendre' }))
  expect(await within(dialog).findByText('Ce compte est clos définitivement.')).toBeDefined()
})

test('a closed account is frozen but can still be deleted, which leads back to its customer', async () => {
  stub({ ...api, status: 'closed' }, { [`DELETE ${path}`]: () => empty(204) })
  const user = userEvent.setup()
  const { router } = renderAt(`/accounts/${api.id}`)
  for (const name of ['Suspendre', 'Réactiver', 'Clôturer']) {
    expect(explained(await screen.findByRole('button', { name }))).toBe('Clos définitivement.')
  }
  for (const block of ['Identité', 'Canaux', 'Sessions', 'Opérations SMPP']) {
    expect(
      explained(within(screen.getByRole('region', { name: block })).getByRole('button', { name: 'Modifier' })),
    ).toBe('Clos définitivement.')
  }
  await user.click(screen.getByRole('button', { name: 'Supprimer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(router.state.location.pathname).toBe(`/customers/${acme.id}`))
})

test('without accounts:write every change is blocked', async () => {
  stub(api, {}, { ...me, permissions: ['accounts:read'] })
  renderAt(`/accounts/${api.id}`)
  for (const name of ['Suspendre', 'Réactiver', 'Clôturer', 'Supprimer']) {
    expect(explained(await screen.findByRole('button', { name }))).toBe(
      'Cette action demande Comptes : modifier accounts:write.',
    )
  }
  for (const block of ['Identité', 'Canaux', 'Sessions', 'Opérations SMPP']) {
    expect(
      explained(within(screen.getByRole('region', { name: block })).getByRole('button', { name: 'Modifier' })),
    ).toBe('Cette action demande Comptes : modifier accounts:write.')
  }
})

test('an active account cannot be reactivated, and its last bind type cannot be removed', async () => {
  stub(api)
  renderAt(`/accounts/${api.id}`)
  expect(explained(await screen.findByRole('button', { name: 'Réactiver' }))).toBe('Déjà actif.')
  const { dialog } = await editBlock('Sessions', false)
  const transceiver = within(dialog).getByRole('checkbox', { name: 'transceiver' })
  expect(transceiver.getAttribute('aria-disabled') ?? transceiver.getAttribute('data-disabled')).not.toBeNull()
  expect(within(dialog).getByRole('checkbox', { name: 'receiver' }).getAttribute('data-disabled')).toBeNull()
})

test('reactivating under an active customer says nothing of the customer', async () => {
  stub({ ...api, status: 'suspended' })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(await screen.findByRole('button', { name: 'Réactiver' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).not.toContain('Son client est suspendu')
})

test('the four flags are each read from their own field', async () => {
  stub({ ...api, smpp_enabled: false, rest_enabled: true, query_sm_enabled: true, cancel_sm_enabled: false })
  renderAt(`/accounts/${api.id}`)
  const value = async (block: string, term: string) =>
    within(await screen.findByRole('region', { name: block })).getByText(term).nextElementSibling?.textContent
  expect([
    await value('Canaux', 'SMPP'),
    await value('Canaux', 'REST'),
    await value('Opérations SMPP', 'query_sm'),
    await value('Opérations SMPP', 'cancel_sm'),
  ]).toEqual(['Désactivé', 'Activé', 'Activé', 'Désactivé'])
})

test('disabling SMPP sends REST alone, and the groups of choices are named', async () => {
  const calls = stub(
    { ...api, rest_enabled: true, query_sm_enabled: false },
    { [`PATCH ${path}/channels`]: () => json(200, api) },
  )
  const { user, dialog } = await editBlock('Canaux')
  expect(within(dialog).getByRole('group', { name: 'Canaux' })).toBeDefined()
  await user.click(within(dialog).getByRole('checkbox', { name: 'SMPP' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/channels')).toEqual({ smpp_enabled: false, rest_enabled: true })
})

test('the last SMPP operation can still be turned off', async () => {
  const calls = stub({ ...api, cancel_sm_enabled: false }, { [`PATCH ${path}/smpp-ops`]: () => json(200, api) })
  const { user, dialog } = await editBlock('Opérations SMPP')
  await user.click(within(dialog).getByRole('checkbox', { name: 'query_sm' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/smpp-ops')).toEqual({ query_sm_enabled: false })
})

test('reactivating or closing an account closed meanwhile is said in French', async () => {
  stub(
    { ...api, status: 'suspended' },
    { [`PATCH ${path}`]: () => json(422, { code: 'validation_error', message: 'not allowed', errors: [] }) },
  )
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  for (const action of ['Réactiver', 'Clôturer']) {
    await user.click(await screen.findByRole('button', { name: action }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: action }))
    expect(await within(dialog).findByText('Ce compte est clos définitivement.')).toBeDefined()
    await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  }
})

test('a rename refused for another reason keeps the gateway message', async () => {
  stub(api, { [`PATCH ${path}`]: () => json(422, { code: 'validation_error', message: 'Nom refusé.', errors: [] }) })
  const { user, dialog } = await editBlock('Identité')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect(await within(dialog).findByRole('alert')).toBeDefined()
  expect(within(dialog).queryByText('Ce client est clos ou n’existe plus.')).toBeNull()
})

test('a refused setting describes the group of choices it concerns', async () => {
  stub(api, {
    [`PATCH ${path}/channels`]: () => json(422, { code: 'validation_error', message: 'Canaux refusés.', errors: [] }),
  })
  const { user, dialog } = await editBlock('Canaux')
  await user.click(within(dialog).getByRole('checkbox', { name: 'REST' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  const alert = await within(dialog).findByRole('alert')
  expect(within(dialog).getByRole('group', { name: 'Canaux' }).getAttribute('aria-describedby')).toBe(alert.id)
})
