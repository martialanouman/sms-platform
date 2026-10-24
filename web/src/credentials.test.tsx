import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import {
  cached,
  empty,
  explained,
  json,
  me,
  noToast,
  renderAt,
  stubFetch,
  toasted,
  understood,
  violations,
} from './test'

const writer = { ...me, permissions: ['accounts:read', 'credentials:read', 'credentials:write'] }
const api = {
  id: '01a16026-0000-7000-8000-0000000000a1',
  customer_id: '01a16026-0000-7000-8000-0000000000c1',
  customer_name: 'Acme',
  customer_status: 'active',
  name: 'api-acme',
  status: 'active',
  smpp_enabled: true,
  rest_enabled: true,
  query_sm_enabled: true,
  cancel_sm_enabled: true,
  allowed_bind_types: ['transceiver'],
  max_sessions: 1,
  rate_limit: null,
  created_at: '2026-10-23T12:00:00Z',
  updated_at: '2026-10-23T12:00:00Z',
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
  created_at: '2026-10-12T12:00:00Z',
  rotated_at: null,
}
const bind = {
  ...key,
  id: '01a16026-0000-7000-8000-0000000000e2',
  type: 'smpp_bind',
  system_id: 'acme01',
  last_used_at: '2026-10-22T12:00:00Z',
  grace_expires_at: '2099-10-24T12:00:00Z',
  rotated_at: '2026-10-21T12:00:00Z',
}

function stub(
  credentials: object[] | (() => object[]),
  rest: Record<string, (r: Request) => Response | Promise<Response>> = {},
  who = writer,
  account: object = api,
) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET /api/gw/admin/accounts/${api.id}`]: () => json(200, account),
    [`GET ${path}`]: () => json(200, typeof credentials === 'function' ? credentials() : credentials),
    ...rest,
  })
}

const card = (name: string) => screen.findByRole('region', { name })
const bindCard = () => card('Bind SMPP')
const keyCard = () => card('Clé d’API')
const value = (region: HTMLElement, label: string): HTMLElement | undefined =>
  [...region.querySelectorAll<HTMLElement>('dt')].find((dt) => dt.textContent === label)?.nextElementSibling as
    | HTMLElement
    | undefined
const sent = async (calls: Request[], method: string) =>
  await calls.find((r) => r.method === method && r.url.includes('/credentials'))?.json()
const reads = (calls: Request[]) => calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length

async function rotation(region: HTMLElement) {
  const user = userEvent.setup()
  await user.click(within(region).getByRole('button', { name: 'Rotation' }))
  return { user, dialog: await screen.findByRole('dialog', { name: /^Rotation/ }) }
}

async function rotate(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement) {
  await user.click(within(dialog).getByRole('checkbox', { name: understood }))
  await user.click(within(dialog).getByRole('button', { name: 'Effectuer la rotation' }))
}

test('the two cards show a filled credential as the mock-up, never a secret', async () => {
  stub([key, bind])
  const { container } = renderAt(`/accounts/${api.id}/credentials`)
  const smpp = await bindCard()
  const rest = await keyCard()
  expect(screen.queryByRole('heading', { name: 'Identifiants' })).toBeNull()
  expect(value(smpp, 'system_id')?.innerHTML).toBe('<span class="ui-mono">acme01</span>')
  expect(value(smpp, 'Dernière utilisation')?.firstElementChild?.className).toBe('ui-mono')
  expect(value(smpp, 'Dernière utilisation')?.textContent).toContain('22/10/2026')
  expect(value(smpp, 'Dernière rotation')?.firstElementChild?.className).toBe('ui-mono')
  expect(value(smpp, 'Dernière rotation')?.textContent).toContain('21/10/2026')
  expect(value(smpp, 'Ancien secret')?.textContent).toMatch(/^accepté jusqu’au 24\/10\/2099 \d\d:\d\d$/)
  expect(value(smpp, 'Ancien secret')?.querySelector('.ui-mono')?.textContent).toMatch(/^24\/10\/2099/)
  expect(value(smpp, 'Créé le')?.firstElementChild?.className).toBe('ui-mono ui-muted')
  expect(value(smpp, 'Créé le')?.textContent).toContain('12/10/2026')
  expect(value(rest, 'system_id')).toBeUndefined()
  expect(value(rest, 'Ancien secret')).toBeUndefined()
  for (const label of ['Dernière utilisation', 'Dernière rotation']) {
    expect(value(rest, label)?.innerHTML).toBe('<span class="ui-muted">jamais</span>')
  }
  for (const region of [smpp, rest]) {
    expect(within(region).getByText('Actif').className).toBe('ui-pill ui-pill--ok')
    expect(within(region).queryByRole('button', { name: /^Créer/ })).toBeNull()
    const foot = within(region).getByRole('button', { name: 'Rotation' }).parentElement
    expect(foot?.className).toBe('ui-card__foot')
    expect(
      within(foot as HTMLElement)
        .getAllByRole('button')
        .map((b) => b.textContent),
    ).toEqual(['Rotation', 'Désactiver', 'Révoquer'])
    expect(within(region).getByRole('button', { name: 'Révoquer' }).className).toContain('ui-button--danger')
  }
  expect(screen.getByRole('region', { name: 'Bind SMPP' }).parentElement?.className).toBe('ui-cards')
  expect(await violations(container)).toEqual([])
})

test('an expired grace is history: no old secret line, no expired date', async () => {
  stub([{ ...bind, grace_expires_at: '2026-01-02T12:00:00Z' }])
  renderAt(`/accounts/${api.id}/credentials`)
  const smpp = await bindCard()
  expect(within(smpp).getByText('system_id')).toBeDefined()
  expect(value(smpp, 'Ancien secret')).toBeUndefined()
  expect(smpp.textContent).not.toContain('02/01/2026')
})

test('a card reads its status in its pill, revoked in the bad tone', async () => {
  stub([
    { ...key, status: 'disabled' },
    { ...bind, status: 'revoked', grace_expires_at: null },
  ])
  renderAt(`/accounts/${api.id}/credentials`)
  expect(within(await keyCard()).getByText('Désactivé').className).toBe('ui-pill ui-pill--neutral')
  expect(within(await bindCard()).getByText('Révoqué').className).toBe('ui-pill ui-pill--bad')
})

test('each empty card says what it is for and offers to create it', async () => {
  stub([])
  const { container } = renderAt(`/accounts/${api.id}/credentials`)
  const smpp = await bindCard()
  const rest = await keyCard()
  expect(within(smpp).getByText('Aucun bind SMPP').className).toBe('ui-card__empty-title')
  expect(within(smpp).getByText(/^Le bind sert au canal SMPP/).textContent).toBe(
    'Le bind sert au canal SMPP. La passerelle génère le mot de passe et le montre une seule fois.',
  )
  expect(within(rest).getByText('Aucune clé d’API').className).toBe('ui-card__empty-title')
  expect(within(rest).getByText(/^La clé sert au canal REST/).textContent).toBe(
    'La clé sert au canal REST. La passerelle la génère et la montre une seule fois.',
  )
  expect(within(smpp).getByRole('button', { name: 'Créer un bind SMPP' }).className).toContain('ui-button--primary')
  expect(within(rest).getByRole('button', { name: 'Créer une clé d’API' }).className).toContain('ui-button--primary')
  for (const region of [smpp, rest]) {
    expect(within(region).queryByRole('button', { name: 'Rotation' })).toBeNull()
    expect(region.querySelector('.ui-pill')).toBeNull()
  }
  expect(await violations(container)).toEqual([])
})

test('a new SMPP bind asks its system_id alone, shows its password once, then the focus goes to the title', async () => {
  let list: object[] = [key]
  const calls = stub(() => list, {
    [`POST ${path}`]: () => {
      list = [key, bind]
      return json(201, { ...bind, secret: 'Ab3dEf7h' }, { 'Cache-Control': 'no-store' })
    },
  })
  const user = userEvent.setup()
  const { queryClient } = renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await bindCard()).getByRole('button', { name: 'Créer un bind SMPP' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau bind SMPP' })
  expect(within(dialog).queryByRole('combobox')).toBeNull()
  const field = within(dialog).getByRole('textbox', { name: 'system_id' }) as HTMLInputElement
  expect([field.required, field.maxLength, field.pattern]).toEqual([true, 15, '[!-~]{1,15}'])
  expect(dialog.textContent).toContain('Le secret est généré par la passerelle et montré une seule fois.')
  await user.type(field, 'acme01')
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  const once = await screen.findByRole('dialog', { name: 'Nouveau mot de passe' })
  expect(within(once).getByText('Ab3dEf7h')).toBeDefined()
  expect(value(once, 'Compte')?.innerHTML).toBe('<span class="ui-mono">api-acme</span>')
  expect(value(once, 'system_id')?.innerHTML).toBe('<span class="ui-mono">acme01</span>')
  expect(await sent(calls, 'POST')).toEqual({ type: 'smpp_bind', system_id: 'acme01' })
  await waitFor(() =>
    expect(within(screen.getByRole('region', { name: 'Bind SMPP', hidden: true })).getByText('acme01')).toBeDefined(),
  )
  expect(cached(queryClient)).not.toContain('Ab3dEf7h')
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'api-acme' }))
  expect(document.body.textContent).not.toContain('Ab3dEf7h')
  expect(reads(calls)).toBe(2)
  noToast()
})

test('a new API key is asked by its button alone and shown once, without a toast', async () => {
  const calls = stub([bind], {
    [`POST ${path}`]: () => json(201, { ...key, secret: `smsgw_${'k'.repeat(43)}` }),
  })
  const user = userEvent.setup()
  const { queryClient } = renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await keyCard()).getByRole('button', { name: 'Créer une clé d’API' }))
  const once = await screen.findByRole('dialog', { name: 'Nouvelle clé d’API' })
  expect(screen.getAllByRole('dialog')).toHaveLength(1)
  expect(within(once).getByText(`smsgw_${'k'.repeat(43)}`)).toBeDefined()
  expect(value(once, 'Compte')?.textContent).toBe('api-acme')
  expect(value(once, 'system_id')).toBeUndefined()
  expect(await sent(calls, 'POST')).toEqual({ type: 'api_key' })
  expect(cached(queryClient)).not.toContain('smsgw_')
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(document.body.textContent).not.toContain('smsgw_')
  noToast()
})

test('a taken system_id and a closed account are said in the creation dialog', async () => {
  let answer = 409
  stub([key], { [`POST ${path}`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await bindCard()).getByRole('button', { name: 'Créer un bind SMPP' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau bind SMPP' })
  await user.type(within(dialog).getByRole('textbox', { name: 'system_id' }), 'acme01')
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce system_id est déjà pris.')
  answer = 422
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  expect(await within(dialog).findByText('Ce compte est fermé ou n’existe plus.')).toBeDefined()
})

test('a key taken meanwhile and a closed account are said in its card', async () => {
  let answer = 409
  stub([bind], { [`POST ${path}`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  const rest = await keyCard()
  await user.click(within(rest).getByRole('button', { name: 'Créer une clé d’API' }))
  expect((await within(rest).findByRole('alert')).textContent).toBe('Ce compte a déjà une clé d’API.')
  answer = 422
  await user.click(within(rest).getByRole('button', { name: 'Créer une clé d’API' }))
  await waitFor(() => expect(within(rest).getByRole('alert').textContent).toBe('Ce compte est fermé ou n’existe plus.'))
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('a bind rotation warns by its grace, waits for the check, sends its grace and gives the focus back', async () => {
  const calls = stub([bind], {
    [`POST ${path}/${bind.id}/rotate`]: () => json(200, { ...bind, secret: 'Zz9yXx8w' }),
  })
  renderAt(`/accounts/${api.id}/credentials`)
  const { user, dialog } = await rotation(await bindCard())
  expect(dialog.querySelector('.ui-modal__title')?.innerHTML).toBe(
    'Rotation du bind SMPP <span class="ui-mono">acme01</span>',
  )
  const banner = () => dialog.querySelector('.ui-banner--warn')?.textContent
  expect(banner()).toBe(
    'Sans grâce, l’ancien mot de passe cesse aussitôt d’être accepté et les binds ouverts du compte sont déconnectés.',
  )
  await user.click(within(dialog).getByRole('combobox', { name: 'Fenêtre de grâce' }))
  expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual(['Aucune', '1 h', '24 h', '7 jours'])
  await user.click(screen.getByRole('option', { name: '24 h' }))
  expect(banner()).toBe('Pendant 24 h, l’ancien mot de passe reste accepté ; aucun bind n’est déconnecté.')
  await rotate(user, dialog)
  const once = await screen.findByRole('dialog', { name: 'Nouveau mot de passe' })
  expect(within(once).getByText('Zz9yXx8w')).toBeDefined()
  expect(await sent(calls, 'POST')).toEqual({ grace_period_sec: 86400 })
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(within(await bindCard()).getByRole('button', { name: 'Rotation' }))
  noToast()
})

test('a key rotation warns by its grace; without one it sends zero, and a disabled key says it stays so', async () => {
  const calls = stub([{ ...key, status: 'disabled' }], {
    [`POST ${path}/${key.id}/rotate`]: () => json(200, { ...key, status: 'disabled', secret: 'smsgw_x' }),
  })
  renderAt(`/accounts/${api.id}/credentials`)
  const { user, dialog } = await rotation(await keyCard())
  expect(dialog.querySelector('.ui-modal__title')?.textContent).toBe('Rotation de la clé d’API')
  const banner = () => dialog.querySelector('.ui-banner--warn')?.textContent
  expect(banner()).toBe('Sans grâce, l’ancienne clé cesse aussitôt d’être acceptée.L’identifiant reste désactivé.')
  await user.click(within(dialog).getByRole('combobox', { name: 'Fenêtre de grâce' }))
  await user.click(await screen.findByRole('option', { name: '1 h' }))
  expect(banner()).toBe('Pendant 1 h, l’ancienne clé reste acceptée.L’identifiant reste désactivé.')
  await user.click(within(dialog).getByRole('combobox', { name: 'Fenêtre de grâce' }))
  await user.click(await screen.findByRole('option', { name: 'Aucune' }))
  await rotate(user, dialog)
  expect(await screen.findByRole('dialog', { name: 'Nouvelle clé d’API' })).toBeDefined()
  expect(await sent(calls, 'POST')).toEqual({ grace_period_sec: 0 })
})

test('a revoked card reads, only rotates, and its rotation has no grace', async () => {
  const revoked = { ...bind, status: 'revoked', grace_expires_at: null }
  const calls = stub([revoked], {
    [`POST ${path}/${bind.id}/rotate`]: () => json(200, { ...revoked, status: 'active', secret: 'Qq1wEe2r' }),
  })
  renderAt(`/accounts/${api.id}/credentials`)
  const smpp = await bindCard()
  expect(value(smpp, 'system_id')?.textContent).toBe('acme01')
  expect(value(smpp, 'Ancien secret')).toBeUndefined()
  expect(within(smpp).queryByRole('button', { name: /^Créer/ })).toBeNull()
  expect(explained(within(smpp).getByRole('button', { name: 'Activer' }))).toBe(
    'Révoqué : seule une rotation le ravive.',
  )
  expect(explained(within(smpp).getByRole('button', { name: 'Révoquer' }))).toBe('Déjà révoqué.')
  expect(within(smpp).getByRole('button', { name: 'Rotation' }).hasAttribute('aria-disabled')).toBe(false)
  const { user, dialog } = await rotation(smpp)
  expect(dialog.querySelector('.ui-banner--warn')?.textContent).toBe(
    'Révoqué : il redevient actif avec un secret neuf, sans grâce.',
  )
  expect(within(dialog).queryByRole('combobox')).toBeNull()
  await rotate(user, dialog)
  expect(await screen.findByRole('dialog', { name: 'Nouveau mot de passe' })).toBeDefined()
  expect(await sent(calls, 'POST')).toEqual({})
})

test('a rotation refused because the account closed meanwhile is said in place', async () => {
  stub([key], {
    [`POST ${path}/${key.id}/rotate`]: () => json(409, { code: 'conflict', message: 'not allowed', errors: [] }),
  })
  renderAt(`/accounts/${api.id}/credentials`)
  const { user, dialog } = await rotation(await keyCard())
  await rotate(user, dialog)
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Ce compte est fermé.')
})

test('disabling, enabling and revoking are confirmed, then told in a toast', async () => {
  const calls = stub([key, { ...bind, status: 'disabled' }], {
    [`PATCH ${path}/${key.id}`]: () => json(200, { ...key, status: 'disabled' }),
    [`PATCH ${path}/${bind.id}`]: () => json(200, { ...bind, status: 'active' }),
    [`DELETE ${path}/${key.id}`]: () => empty(204),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await keyCard()).getByRole('button', { name: 'Désactiver' }))
  const keyOff = await screen.findByRole('alertdialog', { name: 'Désactiver : Clé d’API' })
  expect(keyOff.textContent).not.toContain('binds')
  await user.click(within(keyOff).getByRole('button', { name: 'Désactiver' }))
  expect(await toasted()).toContain('Clé d’API désactivée.')
  expect(await sent(calls, 'PATCH')).toEqual({ status: 'disabled' })
  await user.click(within(await bindCard()).getByRole('button', { name: 'Activer' }))
  const bindOn = await screen.findByRole('alertdialog', { name: 'Activer : Bind SMPP acme01' })
  expect(bindOn.textContent).not.toContain('binds')
  await user.click(within(bindOn).getByRole('button', { name: 'Activer' }))
  await waitFor(async () => expect(await toasted()).toContain('Bind SMPP activé.'))
  expect(await calls.filter((r) => r.method === 'PATCH')[1]?.json()).toEqual({ status: 'active' })
  await user.click(within(await keyCard()).getByRole('button', { name: 'Révoquer' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain('Le secret est effacé ; seule une rotation ravive l’identifiant.')
  await user.click(within(dialog).getByRole('button', { name: 'Révoquer' }))
  await waitFor(async () => expect(await toasted()).toContain('Clé d’API révoquée.'))
  expect(calls.some((r) => r.method === 'DELETE')).toBe(true)
})

test('disabling or revoking a bind says what it does to its binds and its system_id', async () => {
  stub([bind])
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await bindCard()).getByRole('button', { name: 'Révoquer' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain(
    'Le system_id acme01 reste réservé à ce compte.',
  )
  await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Annuler' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  await user.click(within(await bindCard()).getByRole('button', { name: 'Désactiver' }))
  expect((await screen.findByRole('alertdialog')).textContent).toContain('Ses binds ouverts sont déconnectés.')
})

test('a credential revoked meanwhile is said in French, without a toast', async () => {
  stub([key], {
    [`PATCH ${path}/${key.id}`]: () => json(409, { code: 'conflict', message: 'revoked', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  await user.click(within(await keyCard()).getByRole('button', { name: 'Désactiver' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Désactiver' }))
  expect(await within(dialog).findByText('Refusé : l’identifiant est révoqué, ou le compte est fermé.')).toBeDefined()
  noToast()
})

test('a closed account freezes every action of both cards', async () => {
  stub([key], {}, writer, { ...api, status: 'closed' })
  renderAt(`/accounts/${api.id}/credentials`)
  const rest = await keyCard()
  for (const name of ['Rotation', 'Désactiver', 'Révoquer']) {
    expect(explained(within(rest).getByRole('button', { name }))).toBe('Fermé définitivement.')
  }
  expect(explained(within(await bindCard()).getByRole('button', { name: 'Créer un bind SMPP' }))).toBe(
    'Fermé définitivement.',
  )
})

test('without credentials:write every change is blocked, and without credentials:read nothing is asked', async () => {
  stub([key], {}, { ...me, permissions: ['accounts:read', 'credentials:read'] })
  const { unmount } = renderAt(`/accounts/${api.id}/credentials`)
  const reason = 'Cette action demande Identifiants : modifier credentials:write.'
  for (const name of ['Rotation', 'Désactiver', 'Révoquer']) {
    expect(explained(within(await keyCard()).getByRole('button', { name }))).toBe(reason)
  }
  expect(explained(within(await bindCard()).getByRole('button', { name: 'Créer un bind SMPP' }))).toBe(reason)
  unmount()
  const calls = stub([key], {}, { ...me, permissions: ['accounts:read'] })
  renderAt(`/accounts/${api.id}/credentials`)
  await waitFor(() =>
    expect(
      screen
        .getAllByText(/^Cet onglet demande/)
        .filter((e) => e.tagName === 'P')
        .map((e) => e.textContent),
    ).toEqual(['Cet onglet demande Identifiants : lire credentials:read.']),
  )
  expect(screen.queryByRole('region', { name: 'Bind SMPP' })).toBeNull()
  expect(screen.queryByRole('region', { name: 'Clé d’API' })).toBeNull()
  expect(calls.some((r) => r.url.includes('/credentials'))).toBe(false)
})

test('credentials that cannot be read are said, with a retry', async () => {
  stub([], { [`GET ${path}`]: () => json(500, { code: 'internal', message: 'boom', errors: [] }) })
  renderAt(`/accounts/${api.id}/credentials`)
  expect(await screen.findByRole('heading', { name: 'Les identifiants n’ont pas pu être chargés' })).toBeDefined()
  expect(screen.queryByRole('region', { name: 'Bind SMPP' })).toBeNull()
})

test('a key being created holds its button busy', async () => {
  let answer: (r: Response) => void = () => {}
  stub([bind], { [`POST ${path}`]: () => new Promise<Response>((resolve) => (answer = resolve)) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/credentials`)
  const create = within(await keyCard()).getByRole('button', { name: 'Créer une clé d’API' })
  expect(create.hasAttribute('aria-busy')).toBe(false)
  await user.click(create)
  await waitFor(() => expect(create.getAttribute('aria-busy')).toBe('true'))
  answer(json(201, { ...key, secret: 'smsgw_y' }))
  expect(await screen.findByRole('dialog', { name: 'Nouvelle clé d’API' })).toBeDefined()
})
