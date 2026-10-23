import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, explained, json, me, renderAt, stubFetch, violations } from './test'

const writer = { ...me, permissions: ['accounts:read', 'credentials:read', 'credentials:write'] }
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
const path = `/api/gw/admin/accounts/${api.id}/credentials`
const key = {
  id: '01a16026-0000-7000-8000-0000000000e1',
  account_id: api.id,
  type: 'api_key',
  system_id: null,
  status: 'active',
  last_used_at: null,
  grace_expires_at: null,
  created_at: '2026-10-23T08:00:00Z',
  rotated_at: null,
}
const bind = {
  ...key,
  id: '01a16026-0000-7000-8000-0000000000e2',
  type: 'smpp_bind',
  system_id: 'acme01',
  grace_expires_at: '2099-10-24T08:00:00Z',
  rotated_at: '2026-10-23T08:00:00Z',
}

function stub(
  credentials: object[],
  rest: Record<string, (r: Request) => Response | Promise<Response>> = {},
  who = writer,
  account: object = api,
) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET /api/gw/admin/accounts/${api.id}`]: () => json(200, account),
    [`GET ${path}`]: () => json(200, credentials),
    ...rest,
  })
}

async function block() {
  return screen.findByRole('region', { name: 'Identifiants' })
}

const row = async (name: RegExp) => within(await block()).findByRole('row', { name })

test('the credentials of an account show their status, grace and rotation, never a secret', async () => {
  stub([key, bind])
  const { container } = renderAt(`/accounts/${api.id}`)
  const keyRow = await row(/Clé d’API/)
  expect(within(keyRow).getByText('Actif')).toBeDefined()
  const bindRow = await row(/Bind SMPP/)
  expect(within(bindRow).getByText('acme01', { exact: false })).toBeDefined()
  expect(bindRow.textContent).toContain('jusqu’au 24/10/2099')
  expect(bindRow.textContent).toContain('23/10/2026')
  expect(explained(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))).toBe(
    'Un identifiant de chaque type au plus.',
  )
  expect(await violations(container)).toEqual([])
})

test('a new SMPP bind shows its password once, then nothing is left of it', async () => {
  const calls = stub([key], {
    [`POST ${path}`]: () => json(201, { ...bind, secret: 'Ab3dEf7h' }, { 'Cache-Control': 'no-store' }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel identifiant' })
  expect(within(dialog).getByRole('combobox', { name: 'Type' }).textContent).toContain('Bind SMPP')
  await user.type(within(dialog).getByRole('textbox', { name: 'system_id' }), 'acme01')
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  const once = await screen.findByRole('dialog', { name: 'Bind SMPP acme01' })
  expect(within(once).getByText('Ab3dEf7h')).toBeDefined()
  expect(once.textContent).toContain('Ce secret ne sera plus jamais affiché.')
  expect(await calls.find((r) => r.method === 'POST' && r.url.includes('/credentials'))?.json()).toEqual({
    type: 'smpp_bind',
    system_id: 'acme01',
  })
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(document.body.textContent).not.toContain('Ab3dEf7h')
  expect(calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length).toBe(2)
})

test('a new API key needs no system_id', async () => {
  const calls = stub([bind], {
    [`POST ${path}`]: () => json(201, { ...key, secret: `smsgw_${'k'.repeat(43)}` }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel identifiant' })
  expect(within(dialog).queryByRole('textbox', { name: 'system_id' })).toBeNull()
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect(await screen.findByRole('dialog', { name: 'Clé d’API' })).toBeDefined()
  expect(await calls.find((r) => r.method === 'POST' && r.url.includes('/credentials'))?.json()).toEqual({
    type: 'api_key',
  })
})

test('a taken system_id and a closed account are said in French', async () => {
  let answer = 409
  stub([key], {
    [`POST ${path}`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel identifiant' })
  await user.type(within(dialog).getByRole('textbox', { name: 'system_id' }), 'acme01')
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect(await within(dialog).findByText('Ce system_id est déjà pris.')).toBeDefined()
  answer = 422
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect(await within(dialog).findByText('Ce compte est clos ou n’existe plus.')).toBeDefined()
})

test('a rotation sends its grace and shows the new secret once', async () => {
  const calls = stub([bind], {
    [`POST ${path}/${bind.id}/rotate`]: () => json(200, { ...bind, secret: 'Zz9yXx8w' }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Bind SMPP/)).getByRole('button', { name: 'Rotation' }))
  const dialog = await screen.findByRole('dialog', { name: 'Rotation du bind SMPP acme01' })
  expect(dialog.textContent).toContain('Sans grâce, les binds ouverts sont déconnectés.')
  await user.click(within(dialog).getByRole('combobox', { name: 'Grâce' }))
  await user.click(await screen.findByRole('option', { name: '24 h' }))
  expect(dialog.textContent).not.toContain('Sans grâce')
  expect(dialog.textContent).not.toContain('L’identifiant reste désactivé.')
  await user.click(within(dialog).getByRole('button', { name: 'Tourner le secret' }))
  expect(within(await screen.findByRole('dialog', { name: 'Bind SMPP acme01' })).getByText('Zz9yXx8w')).toBeDefined()
  expect(await calls.find((r) => r.method === 'POST' && r.url.includes('/credentials'))?.json()).toEqual({
    grace_period_sec: 86400,
  })
})

test('a rotation without grace sends none, and a disabled credential says it stays so', async () => {
  const calls = stub([{ ...key, status: 'disabled' }], {
    [`POST ${path}/${key.id}/rotate`]: () => json(200, { ...key, status: 'disabled', secret: 'smsgw_x' }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Clé d’API/)).getByRole('button', { name: 'Rotation' }))
  const dialog = await screen.findByRole('dialog', { name: 'Rotation de la clé d’API' })
  expect(dialog.textContent).toContain('L’identifiant reste désactivé.')
  expect(dialog.textContent).not.toContain('binds ouverts')
  await user.click(within(dialog).getByRole('button', { name: 'Tourner le secret' }))
  await screen.findByRole('dialog', { name: 'Clé d’API' })
  expect(await calls.find((r) => r.method === 'POST' && r.url.includes('/credentials'))?.json()).toEqual({
    grace_period_sec: 0,
  })
})

test('disabling, enabling and revoking are confirmed, and a revoked credential only rotates', async () => {
  const calls = stub([key, { ...bind, status: 'revoked', grace_expires_at: null }], {
    [`PATCH ${path}/${key.id}`]: () => json(200, { ...key, status: 'disabled' }),
    [`DELETE ${path}/${key.id}`]: () => empty(204),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  const revoked = await row(/Bind SMPP/)
  expect(explained(within(revoked).getByRole('button', { name: 'Activer' }))).toBe(
    'Révoqué : seule une rotation le ravive.',
  )
  expect(explained(within(revoked).getByRole('button', { name: 'Révoquer' }))).toBe('Déjà révoqué.')
  await user.click(within(await row(/Clé d’API/)).getByRole('button', { name: 'Désactiver' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Désactiver' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await user.click(within(await row(/Clé d’API/)).getByRole('button', { name: 'Révoquer' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain('Le secret est effacé ; seule une rotation ravive l’identifiant.')
  await user.click(within(dialog).getByRole('button', { name: 'Révoquer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({ status: 'disabled' })
  expect(calls.some((r) => r.method === 'DELETE')).toBe(true)
})

test('a credential revoked meanwhile is said in French', async () => {
  stub([key], {
    [`PATCH ${path}/${key.id}`]: () => json(409, { code: 'conflict', message: 'revoked', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Clé d’API/)).getByRole('button', { name: 'Désactiver' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Désactiver' }))
  expect(await within(dialog).findByText('Révoqué : seule une rotation le ravive.')).toBeDefined()
})

test('a closed account freezes its credentials', async () => {
  stub([key], {}, writer, { ...api, status: 'closed' })
  renderAt(`/accounts/${api.id}`)
  const keyRow = await row(/Clé d’API/)
  for (const name of ['Rotation', 'Désactiver', 'Révoquer']) {
    expect(explained(within(keyRow).getByRole('button', { name }))).toBe('Clos définitivement.')
  }
  expect(explained(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))).toBe(
    'Clos définitivement.',
  )
})

test('without credentials:write every change is blocked, and without credentials:read nothing is asked', async () => {
  stub([key], {}, { ...me, permissions: ['accounts:read', 'credentials:read'] })
  const { unmount } = renderAt(`/accounts/${api.id}`)
  expect(explained(within(await row(/Clé d’API/)).getByRole('button', { name: 'Rotation' }))).toBe(
    'Cette action demande credentials:write.',
  )
  unmount()
  const calls = stub([key], {}, { ...me, permissions: ['accounts:read'] })
  renderAt(`/accounts/${api.id}`)
  expect(within(await block()).getByText('Les identifiants demandent credentials:read.')).toBeDefined()
  expect(calls.some((r) => r.url.includes('/credentials'))).toBe(false)
})

test('a disabled credential is enabled, and an expired grace says so', async () => {
  const calls = stub([{ ...key, status: 'disabled', grace_expires_at: '2026-01-02T08:00:00Z' }], {
    [`PATCH ${path}/${key.id}`]: () => json(200, key),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  const keyRow = await row(/Clé d’API/)
  expect(keyRow.textContent).toContain('expirée le 02/01/2026')
  expect(within(keyRow).getByText('Désactivé')).toBeDefined()
  await user.click(within(keyRow).getByRole('button', { name: 'Activer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Activer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await calls.find((r) => r.method === 'PATCH')?.json()).toEqual({ status: 'active' })
})

test('a type taken meanwhile is said in French', async () => {
  stub([bind], { [`POST ${path}`]: () => json(409, { code: 'conflict', message: 'taken', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await block()).getByRole('button', { name: 'Nouvel identifiant' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel identifiant' })
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect(await within(dialog).findByText('Ce compte a déjà un identifiant de ce type.')).toBeDefined()
})

test('a revoked credential rotates without grace, and revoking or disabling a bind says what it does', async () => {
  stub([{ ...key, status: 'revoked' }, bind])
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}`)
  await user.click(within(await row(/Clé d’API/)).getByRole('button', { name: 'Rotation' }))
  const rotation = await screen.findByRole('dialog')
  expect(rotation.textContent).toContain('Révoqué : il redevient actif avec un secret neuf, sans grâce.')
  expect(within(rotation).queryByRole('combobox', { name: 'Grâce' })).toBeNull()
  await user.click(within(rotation).getByRole('button', { name: 'Annuler' }))
  await user.click(within(await row(/Bind SMPP/)).getByRole('button', { name: 'Révoquer' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain(
    'Le system_id acme01 reste réservé à ce compte.',
  )
  await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Annuler' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await user.click(within(await row(/Bind SMPP/)).getByRole('button', { name: 'Désactiver' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain('Ses binds ouverts sont déconnectés.')
})

test('credentials that cannot be read are said, with a retry', async () => {
  stub([], { [`GET ${path}`]: () => json(500, { code: 'internal', message: 'boom', errors: [] }) })
  renderAt(`/accounts/${api.id}`)
  expect(await within(await block()).findByText('Les identifiants n’ont pas pu être chargés')).toBeDefined()
})
