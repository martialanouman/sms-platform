import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, explained, json, me, renderAt, stubFetch, violations } from './test'

const writer = { ...me, permissions: ['accounts:read', 'accounts:write'] }
const api = {
  id: '01a16026-0000-7000-8000-0000000000a1',
  customer_id: '01a16026-0000-7000-8000-0000000000c1',
  customer_name: 'Acme',
  customer_status: 'active',
  name: 'api',
  status: 'active',
  smpp_enabled: true,
  rest_enabled: true,
  query_sm_enabled: true,
  cancel_sm_enabled: true,
  allowed_bind_types: ['transceiver'],
  max_sessions: 1,
  rate_limit: null,
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
}
const path = `/api/gw/admin/accounts/${api.id}/webhooks`
const mo = {
  id: '01a16026-0000-7000-8000-0000000000f1',
  account_id: api.id,
  event_type: 'mo',
  url: 'https://hooks.example.com/mo',
  status: 'active',
  created_at: '2026-10-23T08:00:00Z',
  updated_at: '2026-10-23T08:00:00Z',
}
const dlr = {
  ...mo,
  id: '01a16026-0000-7000-8000-0000000000f2',
  event_type: 'dlr',
  url: 'https://hooks.example.com/dlr',
}
const secret = 'a'.repeat(64)

function stub(
  webhooks: object[],
  rest: Record<string, (r: Request) => Response | Promise<Response>> = {},
  who = writer,
  account: object = api,
) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET /api/gw/admin/accounts/${api.id}`]: () => json(200, account),
    [`GET ${path}`]: () => json(200, webhooks),
    ...rest,
  })
}

const block = () => screen.findByRole('region', { name: 'Webhooks' })
const row = async (name: RegExp) => within(await block()).findByRole('row', { name })
const sent = async (calls: Request[], method: string) =>
  await calls.find((r) => r.method === method && r.url.includes('/webhooks'))?.json()

test('the webhooks of an account show their event, URL and status, never a secret', async () => {
  stub([dlr, { ...mo, status: 'disabled' }])
  const { container } = renderAt(`/accounts/${api.id}`)
  const moRow = await row(/Messages entrants/)
  expect(moRow.textContent).toContain('https://hooks.example.com/mo')
  expect(within(moRow).getByText('Désactivé')).toBeDefined()
  expect(within(await row(/Accusés de réception/)).getByText('Actif')).toBeDefined()
  expect(explained(within(await block()).getByRole('button', { name: 'Nouveau webhook' }))).toBe(
    'Un webhook par événement au plus.',
  )
  expect(await violations(container)).toEqual([])
})

test('a new webhook shows its secret once, then nothing is left of it', async () => {
  const calls = stub([mo], { [`POST ${path}`]: () => json(201, { ...dlr, secret }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouveau webhook' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau webhook' })
  expect(within(dialog).getByRole('combobox', { name: 'Événement' }).textContent).toContain('Accusés de réception')
  await user.type(within(dialog).getByRole('textbox', { name: 'URL' }), 'https://hooks.example.com/dlr')
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  const once = await screen.findByRole('dialog', { name: 'Webhook Accusés de réception (DLR)' })
  expect(within(once).getByText(secret)).toBeDefined()
  expect(once.textContent).toContain('signe chaque appel en HMAC-SHA256')
  expect(await sent(calls, 'POST')).toEqual({ event_type: 'dlr', url: 'https://hooks.example.com/dlr' })
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(document.body.textContent).not.toContain(secret)
  expect(calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length).toBe(2)
})

test('a taken event, a closed account and an unavailable sealing are said in French', async () => {
  let answer = 409
  stub([], { [`POST ${path}`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouveau webhook' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau webhook' })
  await user.type(within(dialog).getByRole('textbox', { name: 'URL' }), 'https://hooks.example.com/mo')
  for (const [status, said] of [
    [409, 'Ce compte a déjà un webhook pour cet événement.'],
    [422, 'Ce compte est clos ou n’existe plus.'],
    [503, 'Le scellement est indisponible : rien n’a été modifié.'],
  ] as const) {
    answer = status
    await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
    expect(await within(dialog).findByText(said)).toBeDefined()
  }
})

test('an edit sends the URL and status that changed', async () => {
  const calls = stub([mo], { [`PATCH ${path}/${mo.id}`]: () => json(200, { ...mo, status: 'disabled' }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Messages entrants/)).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier le webhook Messages entrants (MO)' })
  await user.click(within(dialog).getByRole('combobox', { name: 'Statut' }))
  await user.click(await screen.findByRole('option', { name: 'Désactivé' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await sent(calls, 'PATCH')).toEqual({ status: 'disabled' })
  expect(calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length).toBe(2)
})

test('an edit refused because the account closed meanwhile is said in French', async () => {
  stub([{ ...mo, status: 'disabled' }], {
    [`PATCH ${path}/${mo.id}`]: () => json(409, { code: 'conflict', message: 'closed', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Messages entrants/)).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog')
  const url = within(dialog).getByRole('textbox', { name: 'URL' })
  await user.clear(url)
  await user.type(url, 'https://hooks.example.com/v2')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect(await within(dialog).findByText('Ce compte est clos.')).toBeDefined()
})

test('a rotation is confirmed, then shows the new secret once', async () => {
  const calls = stub([mo], { [`POST ${path}/${mo.id}/rotate-secret`]: () => json(200, { ...mo, secret }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Messages entrants/)).getByRole('button', { name: 'Rotation du secret' }))
  const dialog = await screen.findByRole('dialog', { name: 'Rotation du secret : Messages entrants (MO)' })
  expect(dialog.textContent).toContain('L’ancien secret cesse immédiatement d’être accepté.')
  await user.click(within(dialog).getByRole('button', { name: 'Tourner le secret' }))
  expect(
    within(await screen.findByRole('dialog', { name: 'Webhook Messages entrants (MO)' })).getByText(secret),
  ).toBeDefined()
  expect(calls.some((r) => r.method === 'POST' && r.url.endsWith('/rotate-secret'))).toBe(true)
})

test('a rotation refused is said in French', async () => {
  let answer = 409
  stub([mo], {
    [`POST ${path}/${mo.id}/rotate-secret`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Messages entrants/)).getByRole('button', { name: 'Rotation du secret' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Tourner le secret' }))
  expect(await within(dialog).findByText('Ce compte est clos.')).toBeDefined()
  answer = 503
  await user.click(within(dialog).getByRole('button', { name: 'Tourner le secret' }))
  expect(await within(dialog).findByText('Le scellement est indisponible : rien n’a été modifié.')).toBeDefined()
})

test('a webhook is deleted after a confirmation', async () => {
  const calls = stub([mo], { [`DELETE ${path}/${mo.id}`]: () => empty(204) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Messages entrants/)).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain('Les messages entrants de ce compte ne sont plus remis par webhook.')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(calls.some((r) => r.method === 'DELETE')).toBe(true)
})

test('a closed account freezes new webhooks and rotations but still edits and deletes', async () => {
  stub([mo], {}, writer, { ...api, status: 'closed' })
  renderAt(`/accounts/${api.id}`)
  const moRow = await row(/Messages entrants/)
  expect(explained(within(moRow).getByRole('button', { name: 'Rotation du secret' }))).toBe('Clos définitivement.')
  expect(explained(within(await block()).getByRole('button', { name: 'Nouveau webhook' }))).toBe('Clos définitivement.')
  expect(within(moRow).getByRole('button', { name: 'Modifier' }).getAttribute('aria-disabled')).toBeNull()
  expect(within(moRow).getByRole('button', { name: 'Supprimer' }).getAttribute('aria-disabled')).toBeNull()
})

test('without accounts:write every change is blocked; an unreadable list is said', async () => {
  stub([mo], {}, { ...me, permissions: ['accounts:read'] })
  const { unmount } = renderAt(`/accounts/${api.id}`)
  for (const name of ['Modifier', 'Rotation du secret', 'Supprimer']) {
    expect(explained(within(await row(/Messages entrants/)).getByRole('button', { name }))).toBe(
      'Cette action demande accounts:write.',
    )
  }
  unmount()
  stub([], { [`GET ${path}`]: () => json(500, { code: 'internal', message: 'boom', errors: [] }) })
  renderAt(`/accounts/${api.id}`)
  expect(await within(await block()).findByText('Les webhooks n’ont pas pu être chargés')).toBeDefined()
})
