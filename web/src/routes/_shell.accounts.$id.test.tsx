import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { atMinute } from '../audit'
import { empty, explained, json, me, renderAt, stubFetch, toasted, violations } from '../test'

const writer = {
  ...me,
  permissions: ['accounts:read', 'accounts:write', 'customers:read', 'credentials:read'],
}
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
const page = `/accounts/${api.id}`

function stub(account: object, rest: Record<string, (r: Request) => Response | Promise<Response>> = {}, who = writer) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET ${path}`]: () => json(200, account),
    [`GET ${path}/credentials`]: () => json(200, []),
    [`GET ${path}/webhooks`]: () => json(200, []),
    [`GET /api/gw/admin/customers/${acme.id}`]: () =>
      json(200, {
        id: acme.id,
        name: 'Acme',
        status: 'active',
        group_id: null,
        max_age_override: { otp: null, transactional: null, marketing: null },
        created_at: '2026-10-23T08:00:00Z',
        updated_at: '2026-10-23T08:00:00Z',
      }),
    'GET /api/gw/admin/accounts': () => json(200, { data: [], next_cursor: null }),
    ...rest,
  })
}

const sent = async (calls: Request[], method: string, suffix = '') =>
  await calls.find((r) => r.method === method && new URL(r.url).pathname === path + suffix)?.json()
const reads = (calls: Request[]) => calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length
const crumbs = async () => within(await screen.findByRole('banner')).findByRole('navigation', { name: 'Fil d’Ariane' })
const header = async () => (await screen.findByRole('heading', { level: 1, name: 'api' })).closest('.page-header')
const card = (name: string) => screen.findByRole('region', { name })
const toggle = async (name: string) => screen.findByRole('switch', { name })
const writeReason = 'Cette action demande Comptes : modifier accounts:write.'

test('the page of an account carries the header of the mock-up and opens on its credentials', async () => {
  stub(api)
  const { container, router } = renderAt(page)
  await waitFor(() => expect(router.state.location.pathname).toBe(`${page}/credentials`))
  expect(router.history.length).toBe(1)
  const title = await screen.findByRole('heading', { level: 1, name: 'api' })
  expect(title.className).toBe('page-header__title page-header__title--mono')
  const head = await header()
  expect(head?.querySelector('.page-header__meta')?.textContent).toBe(
    `Compte de Acme · créé le ${atMinute(api.created_at)} · Renommer`,
  )
  const meta = head?.querySelector('.page-header__meta') as HTMLElement
  expect(within(meta).getByRole('link', { name: 'Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`)
  expect(within(meta).getByText(atMinute(api.created_at)).className).toBe('ui-mono')
  expect(within(head as HTMLElement).getByText('Actif').className).toBe('ui-pill ui-pill--ok')
  const nav = await crumbs()
  expect(nav.textContent).toBe('ClientsAcmeapi')
  expect(within(nav).getByRole('link', { name: 'Clients' }).getAttribute('href')).toBe('/customers')
  expect(within(nav).getByRole('link', { name: 'Acme' }).getAttribute('href')).toBe(`/customers/${acme.id}`)
  expect(within(nav).getByText('api').className).toBe('ui-mono')
  const tabs = screen.getByRole('navigation', { name: 'Sections du compte' })
  expect(
    within(tabs)
      .getAllByRole('link')
      .map((l) => [l.textContent, l.getAttribute('href')]),
  ).toEqual([
    ['Identifiants', `${page}/credentials`],
    ['Canaux & opérations', `${page}/channels`],
    ['Sessions', `${page}/sessions`],
    ['Webhooks MO/DLR', `${page}/webhooks`],
  ])
  await waitFor(() =>
    expect(within(tabs).getByRole('link', { name: 'Identifiants' }).getAttribute('aria-current')).toBe('page'),
  )
  expect(await card('Identifiants')).toBeDefined()
  expect(screen.queryByRole('link', { name: '← Acme' })).toBeNull()
  expect(await violations(container)).toEqual([])
})

test('without credentials:read the page opens on its channels, and Identifiants is blocked and explained', async () => {
  stub(api, {}, { ...writer, permissions: ['accounts:read', 'accounts:write'] })
  const { router } = renderAt(page)
  await waitFor(() => expect(router.state.location.pathname).toBe(`${page}/channels`))
  expect(router.history.length).toBe(1)
  const tabs = await screen.findByRole('navigation', { name: 'Sections du compte' })
  const blocked = within(tabs).getByRole('button', { name: 'Identifiants' })
  expect(blocked.hasAttribute('href')).toBe(false)
  expect(explained(blocked)).toBe('Cet onglet demande Identifiants : lire credentials:read.')
  expect(within(tabs).queryByRole('link', { name: 'Identifiants' })).toBeNull()
})

test('the credentials typed in the address without credentials:read mark their blocked tab current', async () => {
  stub(api, {}, { ...writer, permissions: ['accounts:read'] })
  renderAt(`${page}/credentials`)
  const tabs = await screen.findByRole('navigation', { name: 'Sections du compte' })
  await waitFor(() =>
    expect(within(tabs).getByRole('button', { name: 'Identifiants' }).getAttribute('aria-current')).toBe('page'),
  )
  expect(within(tabs).getByRole('link', { name: 'Sessions' }).hasAttribute('aria-current')).toBe(false)
})

test('without customers:read the customer is named, never linked', async () => {
  stub(api, {}, { ...writer, permissions: ['accounts:read'] })
  renderAt(`${page}/channels`)
  const head = (await header()) as HTMLElement
  expect(head.querySelector('.page-header__meta')?.textContent).toContain('Compte de Acme')
  expect(within(head).queryByRole('link')).toBeNull()
  const nav = await crumbs()
  await waitFor(() => expect(nav.textContent).toBe('ClientsAcmeapi'))
  expect(within(nav).queryByRole('link')).toBeNull()
})

test('without accounts:read the page says so and shows no tab', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['customers:read'] }) })
  renderAt(page)
  expect(await screen.findByRole('heading', { name: 'Accès aux comptes non accordé' })).toBeDefined()
  expect(screen.queryByRole('navigation', { name: 'Sections du compte' })).toBeNull()
  const nav = await crumbs()
  await waitFor(() => expect(nav.textContent).toBe('ClientsCompte'))
  expect(within(nav).getByRole('link', { name: 'Clients' })).toBeDefined()
})

test('an account under a suspended customer is shown suspended in effect', async () => {
  stub({ ...api, customer_status: 'suspended' })
  renderAt(`${page}/channels`)
  const head = (await header()) as HTMLElement
  expect(within(head).getByText('Suspendu').className).toBe('ui-pill ui-pill--warn')
  expect(within(head).getByText('par son client').className).toBe('ui-why')
})

test('suspending, reactivating under a suspended customer, and closing are each told in a toast', async () => {
  let current: object = { ...api, status: 'suspended', customer_status: 'suspended' }
  const calls = stub(api, {
    [`GET ${path}`]: () => json(200, current),
    [`PATCH ${path}`]: async (r) => {
      current = { ...current, ...(await r.clone().json()) }
      return json(200, current)
    },
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await screen.findByRole('button', { name: 'Réactiver' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Réactiver le compte api' })
  expect(dialog.textContent).toContain('Son client est suspendu : le compte reste suspendu en pratique.')
  await user.click(within(dialog).getByRole('button', { name: 'Réactiver' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await toasted()).toContain('Compte api réactivé.')
  await user.click(screen.getByRole('button', { name: 'Clôturer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Clôturer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await toasted()).toContain('Compte api fermé.')
  const patches = calls.filter((r) => r.method === 'PATCH')
  expect(await Promise.all(patches.map((r) => r.json()))).toEqual([{ status: 'active' }, { status: 'closed' }])
})

test('suspending by the keyboard is told in a toast and leaves the focus on Suspendre', async () => {
  let current: object = api
  stub(api, {
    [`GET ${path}`]: () => json(200, current),
    [`POST ${path}/suspend`]: () => {
      current = { ...api, status: 'suspended' }
      return json(200, current)
    },
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const suspend = await screen.findByRole('button', { name: 'Suspendre' })
  suspend.focus()
  await user.keyboard('{Enter}')
  const dialog = await screen.findByRole('alertdialog')
  expect(dialog.textContent).toContain('Ses binds ouverts sont déconnectés.')
  within(dialog).getByRole('button', { name: 'Suspendre' }).focus()
  await user.keyboard('{Enter}')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await toasted()).toEqual(['Compte api suspendu.'])
  await waitFor(() => expect(explained(screen.getByRole('button', { name: 'Suspendre' }))).toBe('Déjà suspendu.'))
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Suspendre' }))
})

test('a suspension refused because the account is closed is said in French', async () => {
  stub(api, { [`POST ${path}/suspend`]: () => json(409, { code: 'conflict', message: 'closed', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await screen.findByRole('button', { name: 'Suspendre' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Suspendre' }))
  expect(await within(dialog).findByText('Ce compte est fermé définitivement.')).toBeDefined()
})

test('reactivating or closing an account closed meanwhile is said in French', async () => {
  stub(
    { ...api, status: 'suspended' },
    { [`PATCH ${path}`]: () => json(422, { code: 'validation_error', message: 'not allowed', errors: [] }) },
  )
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  for (const action of ['Réactiver', 'Clôturer']) {
    await user.click(await screen.findByRole('button', { name: action }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: action }))
    expect(await within(dialog).findByText('Ce compte est fermé définitivement.')).toBeDefined()
    await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  }
})

test('reactivating under an active customer says nothing of the customer', async () => {
  stub({ ...api, status: 'suspended' })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await screen.findByRole('button', { name: 'Réactiver' }))
  expect((await screen.findByRole('alertdialog')).textContent).not.toContain('Son client est suspendu')
})

test('a closed account is frozen but can still be deleted, which leads to its customer with a toast', async () => {
  stub({ ...api, status: 'closed' }, { [`DELETE ${path}`]: () => empty(204) })
  const user = userEvent.setup()
  const { router } = renderAt(`${page}/channels`)
  for (const name of ['Suspendre', 'Réactiver', 'Clôturer', 'Renommer']) {
    expect(explained(await screen.findByRole('button', { name }))).toBe('Fermé définitivement.')
  }
  await user.click(screen.getByRole('button', { name: 'Supprimer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(router.state.location.pathname).toBe(`/customers/${acme.id}`))
  expect(await toasted()).toEqual(['Compte api supprimé.'])
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Acme' })))
})

test('without accounts:write every change is blocked', async () => {
  stub(api, {}, { ...writer, permissions: ['accounts:read'] })
  renderAt(`${page}/channels`)
  for (const name of ['Suspendre', 'Réactiver', 'Clôturer', 'Supprimer', 'Renommer']) {
    expect(explained(await screen.findByRole('button', { name }))).toBe(writeReason)
  }
  for (const name of ['Canal SMPP', 'Canal REST', 'query_sm autorisé', 'cancel_sm autorisé']) {
    expect(explained(await toggle(name))).toBe(writeReason)
  }
})

test('an active account cannot be reactivated', async () => {
  stub(api)
  renderAt(`${page}/channels`)
  expect(explained(await screen.findByRole('button', { name: 'Réactiver' }))).toBe('Déjà actif.')
})

test('a rename onto a taken name is refused in French', async () => {
  const calls = stub(api, {
    [`PATCH ${path}`]: () => json(409, { code: 'conflict', message: 'already taken', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await screen.findByRole('button', { name: 'Renommer' }))
  const dialog = await screen.findByRole('dialog')
  const name = within(dialog).getByRole('textbox', { name: 'Nom' })
  await user.clear(name)
  await user.type(name, 'web')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect(await within(dialog).findByText('Un compte de ce client porte déjà ce nom.')).toBeDefined()
  expect(await sent(calls, 'PATCH')).toEqual({ name: 'web' })
})

test('a rename that succeeds is told in a toast', async () => {
  stub(api, { [`PATCH ${path}`]: () => json(200, { ...api, name: 'web' }) })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await screen.findByRole('button', { name: 'Renommer' }))
  const dialog = await screen.findByRole('dialog')
  const name = within(dialog).getByRole('textbox', { name: 'Nom' })
  await user.clear(name)
  await user.type(name, 'web')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(await toasted()).toEqual(['Compte web renommé.'])
})

test('the channels and SMPP operations are switches read from their own fields', async () => {
  stub({ ...api, smpp_enabled: false, rest_enabled: true, query_sm_enabled: true, cancel_sm_enabled: false })
  const { container } = renderAt(`${page}/channels`)
  const state = async (name: string) => (await toggle(name)).getAttribute('aria-checked')
  expect([
    await state('Canal SMPP'),
    await state('Canal REST'),
    await state('query_sm autorisé'),
    await state('cancel_sm autorisé'),
  ]).toEqual(['false', 'true', 'true', 'false'])
  expect(within(await card('Canaux')).getByText('s’appliquent immédiatement')).toBeDefined()
  expect(within(await card('Opérations SMPP')).getByText('confirmées : elles se lisent au bind')).toBeDefined()
  expect(explained(await toggle('Canal REST'))).toBe('Au moins un canal reste activé.')
  expect(explained(await toggle('Canal SMPP'))).toBe(
    'Les binds SMPP de ce compte ne sont acceptés que si ce canal est activé.',
  )
  expect(explained(await toggle('cancel_sm autorisé'))).toBe('Le changement déconnecte les binds ouverts du compte.')
  expect(
    within(await toggle('cancel_sm autorisé').then((t) => t.closest('.ui-switch') as HTMLElement)).getByText(
      'cancel_sm',
    ).className,
  ).toBe('ui-mono')
  expect(await violations(container)).toEqual([])
})

test('a channel switch applies at once, takes the answer as the account, reads the accounts again, and is told in a toast', async () => {
  let release = () => {}
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  let served = 0
  const calls = stub(api, {
    [`GET ${path}`]: () => {
      served += 1
      return served === 1 ? json(200, api) : new Promise<Response>(() => {})
    },
    [`PATCH ${path}/channels`]: async () => {
      await held
      return json(200, { ...api, rest_enabled: true })
    },
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const smpp = await toggle('Canal SMPP')
  expect(explained(smpp)).toBe('Au moins un canal reste activé.')
  const rest = await toggle('Canal REST')
  await user.click(rest)
  await waitFor(() => expect(rest.getAttribute('aria-busy')).toBe('true'))
  expect(rest.getAttribute('aria-readonly')).toBe('true')
  expect(screen.queryByRole('alertdialog')).toBeNull()
  release()
  await waitFor(() => expect(rest.getAttribute('aria-checked')).toBe('true'))
  expect(rest.hasAttribute('aria-busy')).toBe(false)
  await waitFor(() => expect(reads(calls)).toBe(2))
  expect(await sent(calls, 'PATCH', '/channels')).toEqual({ smpp_enabled: true, rest_enabled: true })
  expect(await toasted()).toEqual(['Canal REST activé.'])
  await waitFor(() => expect(smpp.hasAttribute('aria-disabled')).toBe(false))
})

test('while a channel is being set, both channel switches are held and a click sends nothing', async () => {
  let release = () => {}
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  const calls = stub(
    { ...api, rest_enabled: true },
    {
      [`PATCH ${path}/channels`]: async () => {
        await held
        return json(200, { ...api, smpp_enabled: false, rest_enabled: true })
      },
    },
  )
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const rest = await toggle('Canal REST')
  await user.click(await toggle('Canal SMPP'))
  await waitFor(() => expect(rest.getAttribute('aria-busy')).toBe('true'))
  expect(rest.getAttribute('aria-readonly')).toBe('true')
  await user.click(rest)
  release()
  expect(await toasted()).toEqual(['Canal SMPP désactivé.'])
  expect(calls.filter((r) => r.method === 'PATCH')).toHaveLength(1)
})

test('disabling one channel sends the other as it is, and is told in a toast', async () => {
  for (const [name, body] of [
    ['SMPP', { smpp_enabled: false, rest_enabled: true }],
    ['REST', { smpp_enabled: true, rest_enabled: false }],
  ] as const) {
    const calls = stub({ ...api, rest_enabled: true }, { [`PATCH ${path}/channels`]: () => json(200, api) })
    const user = userEvent.setup()
    const { unmount } = renderAt(`${page}/channels`)
    await user.click(await toggle(`Canal ${name}`))
    expect(await toasted()).toEqual([`Canal ${name} désactivé.`])
    expect(await sent(calls, 'PATCH', '/channels')).toEqual(body)
    unmount()
  }
})

test('a refused channel is said in its card, the switch keeps its state and the account is read again', async () => {
  const calls = stub(
    { ...api, rest_enabled: true },
    { [`PATCH ${path}/channels`]: () => json(422, { code: 'validation_error', message: 'not allowed', errors: [] }) },
  )
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const smpp = await toggle('Canal SMPP')
  await waitFor(() => expect(reads(calls)).toBe(1))
  await user.click(smpp)
  const alert = await within(await card('Canaux')).findByRole('alert')
  expect(alert.textContent).toBe('Au moins un canal reste activé.')
  expect(smpp.getAttribute('aria-checked')).toBe('true')
  await waitFor(() => expect(reads(calls)).toBe(2))
  expect(screen.getByRole('region', { name: 'Notifications' }).querySelectorAll('.ui-toast')).toHaveLength(0)
})

test('a channel refused for another reason keeps the gateway message', async () => {
  stub(api, {
    [`PATCH ${path}/channels`]: () => json(503, { code: 'unavailable', message: 'Service indisponible.', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  await user.click(await toggle('Canal REST'))
  expect((await within(await card('Canaux')).findByRole('alert')).textContent).toBe('Service indisponible.')
})

test('an SMPP operation switch asks first, and Annuler sends nothing', async () => {
  const calls = stub(api)
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const query = await toggle('query_sm autorisé')
  await user.click(query)
  const dialog = await screen.findByRole('alertdialog', { name: 'Interdire query_sm' })
  expect(dialog.textContent).toContain('Les drapeaux se lisent au bind : les binds ouverts du compte sont déconnectés.')
  expect(within(dialog).getByRole('button', { name: 'Interdire' })).toBeDefined()
  await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(query.getAttribute('aria-checked')).toBe('true')
  expect(calls.some((r) => r.method === 'PATCH')).toBe(false)
})

test('a confirmed SMPP operation sends only its flag and is told in a toast', async () => {
  let current: object = api
  const calls = stub(api, {
    [`GET ${path}`]: () => json(200, current),
    [`PATCH ${path}/smpp-ops`]: () => {
      current = { ...api, cancel_sm_enabled: true }
      return json(200, current)
    },
  })
  const user = userEvent.setup()
  renderAt(`${page}/channels`)
  const cancel = await toggle('cancel_sm autorisé')
  await user.click(cancel)
  const dialog = await screen.findByRole('alertdialog', { name: 'Autoriser cancel_sm' })
  await user.click(within(dialog).getByRole('button', { name: 'Autoriser' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(await sent(calls, 'PATCH', '/smpp-ops')).toEqual({ cancel_sm_enabled: true })
  expect(await toasted()).toEqual(['cancel_sm autorisé.'])
  await waitFor(() => expect(cancel.getAttribute('aria-checked')).toBe('true'))
})

test('a closed account freezes its switches', async () => {
  stub({ ...api, status: 'closed' })
  renderAt(`${page}/channels`)
  for (const name of ['Canal SMPP', 'Canal REST', 'query_sm autorisé', 'cancel_sm autorisé']) {
    expect(explained(await toggle(name))).toBe('Fermé définitivement.')
  }
})

test('the sessions tab shows its limits in place and the live binds as not yet in service', async () => {
  stub({ ...api, allowed_bind_types: ['transmitter', 'transceiver'] })
  const { container } = renderAt(`${page}/sessions`)
  const limits = await card('Limites de session')
  const max = within(limits).getByRole('spinbutton', { name: 'max_sessions' })
  expect((max as HTMLInputElement).value).toBe('2')
  expect(max.getAttribute('max')).toBe('2147483647')
  expect(explained(max)).toBe('Baisser la limite ne coupe aucun bind ouvert.')
  const types = within(limits).getByRole('group', { name: 'Types de bind' })
  expect(
    within(types)
      .getAllByRole('checkbox')
      .map((c) => c.getAttribute('aria-checked')),
  ).toEqual(['true', 'false', 'true'])
  expect(within(types).getByRole('checkbox', { name: 'Émetteur transmitter' }).getAttribute('data-disabled')).toBeNull()
  expect(within(types).getByText('transmitter').className).toBe('ui-mono ui-muted')
  expect(max.className).toContain('ui-mono')
  expect(limits.textContent).toContain('Au moins un type reste autorisé.')
  const live = await card('Binds vivants')
  expect(live.querySelector('.ui-card__head .ui-why')?.textContent).toBe('limite 2')
  expect(within(live).getByText('2').className).toBe('ui-mono')
  expect(live.textContent).toContain('Sessions en direct indisponibles')
  expect(await violations(container)).toEqual([])
})

test('the session limits are sent with the bind types in a fixed order and told in a toast', async () => {
  let served = 0
  const calls = stub(api, {
    [`GET ${path}`]: () => {
      served += 1
      return served === 1 ? json(200, api) : new Promise<Response>(() => {})
    },
    [`PATCH ${path}/session-limits`]: () => json(200, { ...api, max_sessions: 0 }),
  })
  const user = userEvent.setup()
  renderAt(`${page}/sessions`)
  const limits = await card('Limites de session')
  const max = within(limits).getByRole('spinbutton', { name: 'max_sessions' })
  await user.clear(max)
  await user.type(max, '0')
  await user.click(within(limits).getByRole('checkbox', { name: 'Récepteur receiver' }))
  await user.click(within(limits).getByRole('checkbox', { name: 'Émetteur transmitter' }))
  await user.click(within(limits).getByRole('button', { name: 'Enregistrer' }))
  expect(await toasted()).toEqual(['Limites de session enregistrées.'])
  expect((await card('Binds vivants')).querySelector('.ui-card__head .ui-why')?.textContent).toBe('limite 0')
  await waitFor(() => expect(reads(calls)).toBe(2))
  expect(await sent(calls, 'PATCH', '/session-limits')).toEqual({
    max_sessions: 0,
    allowed_bind_types: ['transmitter', 'receiver', 'transceiver'],
  })
})

test('the last bind type cannot be removed, and a refused limit is said in its card', async () => {
  stub(api, {
    [`PATCH ${path}/session-limits`]: () =>
      json(422, { code: 'validation_error', message: 'Limite refusée.', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`${page}/sessions`)
  const limits = await card('Limites de session')
  const transceiver = within(limits).getByRole('checkbox', { name: 'Émetteur-récepteur transceiver' })
  expect(transceiver.getAttribute('data-disabled')).not.toBeNull()
  expect(within(limits).getByRole('checkbox', { name: 'Récepteur receiver' }).getAttribute('data-disabled')).toBeNull()
  await user.click(within(limits).getByRole('button', { name: 'Enregistrer' }))
  const alert = await within(limits).findByRole('alert')
  expect(alert.textContent).toBe('Limite refusée.')
  expect(screen.getByRole('region', { name: 'Notifications' }).querySelectorAll('.ui-toast')).toHaveLength(0)
  expect(within(limits).getByRole('group', { name: 'Types de bind' }).getAttribute('aria-describedby')).toBe(alert.id)
  expect(fireEvent.submit(limits.querySelector('form') as HTMLFormElement)).toBe(false)
})

test('the session limits are frozen on a closed account and without accounts:write', async () => {
  stub({ ...api, status: 'closed' })
  const { unmount } = renderAt(`${page}/sessions`)
  const limits = await card('Limites de session')
  expect(explained(within(limits).getByRole('button', { name: 'Enregistrer' }))).toBe('Fermé définitivement.')
  expect(within(limits).getByRole('spinbutton', { name: 'max_sessions' }).hasAttribute('readonly')).toBe(true)
  expect(
    within(limits).getByRole('checkbox', { name: 'Récepteur receiver' }).getAttribute('data-disabled'),
  ).not.toBeNull()
  unmount()
  stub(api, {}, { ...writer, permissions: ['accounts:read'] })
  renderAt(`${page}/sessions`)
  expect(explained(within(await card('Limites de session')).getByRole('button', { name: 'Enregistrer' }))).toBe(
    writeReason,
  )
})
