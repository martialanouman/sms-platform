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

const writer = { ...me, permissions: ['accounts:read', 'accounts:write'] }
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
  webhooks: object[] | (() => object[]),
  rest: Record<string, (r: Request) => Response | Promise<Response>> = {},
  who = writer,
  account: object = api,
) {
  return stubFetch({
    'GET /api/auth/me': () => json(200, who),
    [`GET /api/gw/admin/accounts/${api.id}`]: () => json(200, account),
    [`GET ${path}`]: () => json(200, typeof webhooks === 'function' ? webhooks() : webhooks),
    ...rest,
  })
}

const block = () => screen.findByRole('region', { name: 'Webhooks MO / DLR' })
const row = async (name: RegExp) => within(await block()).findByRole('row', { name })
const add = async () => within(await block()).findByRole('button', { name: 'Ajouter' })
const sent = async (calls: Request[], method: string) =>
  await calls.find((r) => r.method === method && r.url.includes('/webhooks'))?.json()
const reads = (calls: Request[]) => calls.filter((r) => r.method === 'GET' && new URL(r.url).pathname === path).length

test('the webhooks read as the mock-up: event badge, mono URL, pill and five actions', async () => {
  stub([mo, { ...dlr, status: 'disabled' }])
  const { container } = renderAt(`/accounts/${api.id}/webhooks`)
  const region = await block()
  expect(region.className).toBe('ui-card ui-card--flush')
  expect(screen.queryByRole('heading', { name: 'Webhooks' })).toBeNull()
  const moRow = await row(/SMS entrants/)
  const [url, status] = within(moRow).getAllByRole('cell')
  expect(within(moRow).getByRole('rowheader').innerHTML).toBe(
    '<span class="ui-badge ui-badge--mo">MO</span><span class="ui-why">SMS entrants</span>',
  )
  expect(url?.innerHTML).toBe('<span class="ui-mono">https://hooks.example.com/mo</span>')
  expect(within(status as HTMLElement).getByText('Actif').className).toBe('ui-pill ui-pill--ok')
  expect(
    within(moRow)
      .getAllByRole('button')
      .map((b) => b.textContent),
  ).toEqual(['Désactiver', 'Modifier l’URL', 'Rotation du secret', 'Supprimer'])
  expect(within(moRow).getByRole('button', { name: 'Supprimer' }).className).toContain('ui-button--danger')
  const actions = within(moRow).getByRole('button', { name: 'Supprimer' }).parentElement
  expect(actions?.className).toBe('row-actions')
  expect(actions?.parentElement?.className).toBe('ui-table__cell ui-table__cell--end')
  expect(within(region).getByRole('columnheader', { name: 'Actions' }).className).toContain('ui-table__cell--end')
  const dlrRow = await row(/accusés de réception/)
  expect(within(dlrRow).getByRole('rowheader').innerHTML).toBe(
    '<span class="ui-badge ui-badge--dlr">DLR</span><span class="ui-why">accusés de réception</span>',
  )
  expect(within(dlrRow).getByText('Désactivé').className).toBe('ui-pill ui-pill--neutral')
  expect(within(dlrRow).getByRole('button', { name: 'Activer' })).toBeDefined()
  const button = await add()
  expect(button.className).toBe('ui-button ui-button--primary ui-button--sm')
  expect(explained(button)).toBe('Un webhook par événement au plus.')
  expect(region.nextElementSibling?.textContent).toBe(
    'Le secret signe chaque appel en HMAC-SHA256 ; il n’est montré qu’à sa création ou à sa rotation, et la rotation coupe l’ancien aussitôt. Chaque action est enregistrée dans le journal d’audit.',
  )
  expect(region.nextElementSibling?.className).toBe('ui-why')
  expect(await violations(container)).toEqual([])
})

test('an account without webhooks says so, and adds one', async () => {
  stub([])
  const { container } = renderAt(`/accounts/${api.id}/webhooks`)
  const region = await block()
  expect(await within(region).findByText('Aucun webhook')).toBeDefined()
  expect(region.textContent).toContain(
    'Les SMS entrants et les accusés de réception de ce compte ne sont remis à aucune URL.',
  )
  expect((await add()).hasAttribute('aria-disabled')).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('a new webhook shows its secret once, then nothing is left of it and the focus is back on Ajouter', async () => {
  const calls = stub([mo], { [`POST ${path}`]: () => json(201, { ...dlr, secret }) })
  const user = userEvent.setup()
  const { queryClient } = renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(await add())
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau webhook' })
  expect(within(dialog).getByRole('combobox', { name: 'Événement' }).textContent).toContain('DLR')
  await user.click(within(dialog).getByRole('combobox', { name: 'Événement' }))
  expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual(['DLR (accusés de réception)'])
  await user.keyboard('{Escape}')
  const url = within(dialog).getByRole('textbox', { name: 'URL' }) as HTMLInputElement
  expect([url.type, url.getAttribute('maxlength')]).toEqual(['url', '2048'])
  for (const bad of ['http://hooks.example.com', 'https://u@hooks.example.com']) {
    await user.type(url, bad)
    expect(url.validity.patternMismatch).toBe(true)
    await user.clear(url)
  }
  await user.type(url, 'https://hooks.example.com/dlr')
  expect(url.validity.patternMismatch).toBe(false)
  await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
  const once = await screen.findByRole('dialog', { name: 'Nouveau secret du webhook DLR' })
  expect(within(once).getByText(secret)).toBeDefined()
  expect(once.textContent).toContain('Ce secret signe chaque appel en HMAC-SHA256.')
  expect(await sent(calls, 'POST')).toEqual({ event_type: 'dlr', url: 'https://hooks.example.com/dlr' })
  expect(cached(queryClient)).not.toContain(secret)
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(await add())
  expect(document.body.textContent).not.toContain(secret)
  await waitFor(() => expect(reads(calls)).toBe(2))
  noToast()
})

test('a taken event, a closed account and an unavailable sealing are said in French', async () => {
  let answer = 409
  stub([], { [`POST ${path}`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(await add())
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau webhook' })
  await user.type(within(dialog).getByRole('textbox', { name: 'URL' }), 'https://hooks.example.com/mo')
  for (const [status, said] of [
    [409, 'Ce compte a déjà un webhook pour cet événement.'],
    [422, 'Ce compte est fermé ou n’existe plus.'],
    [503, 'Le scellement est indisponible : rien n’a été modifié.'],
  ] as const) {
    answer = status
    await user.click(within(dialog).getByRole('button', { name: 'Créer' }))
    expect(await within(dialog).findByText(said)).toBeDefined()
  }
})

test('disabling and enabling are confirmed, send the status alone and are told in a toast', async () => {
  const calls = stub([mo, { ...dlr, status: 'disabled' }], {
    [`PATCH ${path}/${mo.id}`]: () => json(200, { ...mo, status: 'disabled' }),
    [`PATCH ${path}/${dlr.id}`]: () => json(200, dlr),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Désactiver' }))
  const off = await screen.findByRole('alertdialog', { name: 'Désactiver le webhook MO' })
  expect(off.textContent).toContain('Les SMS entrants de ce compte ne sont plus remis par webhook.')
  await user.click(within(off).getByRole('button', { name: 'Désactiver' }))
  expect(await toasted()).toContain('Webhook MO désactivé.')
  expect(await sent(calls, 'PATCH')).toEqual({ status: 'disabled' })
  await user.click(within(await row(/accusés de réception/)).getByRole('button', { name: 'Activer' }))
  const on = await screen.findByRole('alertdialog', { name: 'Activer le webhook DLR' })
  expect(on.textContent).toContain('Les accusés de réception de ce compte sont de nouveau remis par webhook.')
  await user.click(within(on).getByRole('button', { name: 'Activer' }))
  await waitFor(async () => expect(await toasted()).toContain('Webhook DLR activé.'))
  expect(await calls.filter((r) => r.method === 'PATCH')[1]?.json()).toEqual({ status: 'active' })
})

test('enabling refused because the account closed meanwhile is said in place', async () => {
  stub([{ ...mo, status: 'disabled' }], {
    [`PATCH ${path}/${mo.id}`]: () => json(409, { code: 'conflict', message: 'closed', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Activer' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Activer' }))
  expect(await within(dialog).findByText('Ce compte est fermé.')).toBeDefined()
  noToast()
})

test('an edit sends the URL alone and is told in a toast', async () => {
  const calls = stub([mo], {
    [`PATCH ${path}/${mo.id}`]: () => json(200, { ...mo, url: 'https://hooks.example.com/v2' }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Modifier l’URL' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier l’URL du webhook MO' })
  expect(within(dialog).queryByRole('combobox')).toBeNull()
  const url = within(dialog).getByRole('textbox', { name: 'URL' })
  await user.clear(url)
  await user.type(url, 'https://hooks.example.com/v2')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect(await toasted()).toContain('URL du webhook MO modifiée.')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(await sent(calls, 'PATCH')).toEqual({ url: 'https://hooks.example.com/v2' })
  await waitFor(() => expect(reads(calls)).toBe(2))
})

test('a rotation goes through the rotation dialog, sends no body and shows the new secret once', async () => {
  const calls = stub([mo], { [`POST ${path}/${mo.id}/rotate-secret`]: () => json(200, { ...mo, secret }) })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  const rotation = within(await row(/SMS entrants/)).getByRole('button', { name: 'Rotation du secret' })
  await user.click(rotation)
  const dialog = await screen.findByRole('dialog', { name: 'Rotation du secret du webhook MO' })
  expect(dialog.querySelector('.ui-banner--warn')?.textContent).toBe(
    'L’ancien secret cesse immédiatement d’être accepté.',
  )
  expect(within(dialog).queryByRole('combobox')).toBeNull()
  await user.click(within(dialog).getByRole('checkbox', { name: understood }))
  await user.click(within(dialog).getByRole('button', { name: 'Effectuer la rotation' }))
  const once = await screen.findByRole('dialog', { name: 'Nouveau secret du webhook MO' })
  expect(within(once).getByText(secret)).toBeDefined()
  const kv = [...once.querySelectorAll('dt')].map((dt) => [dt.textContent, dt.nextElementSibling?.textContent])
  expect(kv).toEqual([
    ['Compte', 'api-acme'],
    ['Événement', 'MO'],
  ])
  expect(await calls.find((r) => r.url.endsWith('/rotate-secret'))?.text()).toBe('')
  await user.click(within(once).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(document.activeElement).toBe(rotation)
  noToast()
})

test('a rotation refused is said in French', async () => {
  let answer = 409
  stub([mo], {
    [`POST ${path}/${mo.id}/rotate-secret`]: () => json(answer, { code: 'x', message: 'refused', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Rotation du secret' }))
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('checkbox', { name: understood }))
  await user.click(within(dialog).getByRole('button', { name: 'Effectuer la rotation' }))
  expect(await within(dialog).findByText('Ce compte est fermé.')).toBeDefined()
  answer = 503
  await user.click(within(dialog).getByRole('button', { name: 'Effectuer la rotation' }))
  expect(await within(dialog).findByText('Le scellement est indisponible : rien n’a été modifié.')).toBeDefined()
})

test('a deleted webhook leaves the table at once, the focus goes to the title, and a toast tells it', async () => {
  let list: object[] = [mo]
  const calls = stub(() => list, {
    [`DELETE ${path}/${mo.id}`]: () => {
      list = []
      return empty(204)
    },
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Supprimer le webhook MO' })
  expect(dialog.textContent).toContain('Les SMS entrants de ce compte ne sont plus remis par webhook.')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  expect(within(await block()).queryByRole('row', { name: /SMS entrants/ })).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'api-acme' }))
  expect(await toasted()).toContain('Webhook MO supprimé.')
  expect(calls.some((r) => r.method === 'DELETE')).toBe(true)
  await waitFor(() => expect(reads(calls)).toBe(2))
})

test('a closed account freezes adding, enabling and rotating, but still disables, edits and deletes', async () => {
  stub([mo, { ...dlr, status: 'disabled' }], {}, writer, { ...api, status: 'closed' })
  renderAt(`/accounts/${api.id}/webhooks`)
  const moRow = await row(/SMS entrants/)
  const dlrRow = await row(/accusés de réception/)
  expect(explained(await add())).toBe('Fermé définitivement.')
  expect(explained(within(dlrRow).getByRole('button', { name: 'Activer' }))).toBe('Fermé définitivement.')
  expect(explained(within(moRow).getByRole('button', { name: 'Rotation du secret' }))).toBe('Fermé définitivement.')
  for (const name of ['Désactiver', 'Modifier l’URL', 'Supprimer']) {
    expect(within(moRow).getByRole('button', { name }).getAttribute('aria-disabled')).toBeNull()
  }
})

test('without accounts:write every change is blocked; an unreadable list is said', async () => {
  stub([mo, { ...dlr, status: 'disabled' }], {}, { ...me, permissions: ['accounts:read'] })
  const { unmount } = renderAt(`/accounts/${api.id}/webhooks`)
  const reason = 'Cette action demande Comptes : modifier accounts:write.'
  for (const name of ['Désactiver', 'Modifier l’URL', 'Rotation du secret', 'Supprimer']) {
    expect(explained(within(await row(/SMS entrants/)).getByRole('button', { name }))).toBe(reason)
  }
  expect(explained(within(await row(/accusés de réception/)).getByRole('button', { name: 'Activer' }))).toBe(reason)
  expect(explained(await add())).toBe(reason)
  unmount()
  stub([], { [`GET ${path}`]: () => json(500, { code: 'internal', message: 'boom', errors: [] }) })
  renderAt(`/accounts/${api.id}/webhooks`)
  expect(await within(await block()).findByText('Les webhooks n’ont pas pu être chargés')).toBeDefined()
  expect(within(await block()).queryByRole('button', { name: 'Ajouter' })).toBeNull()
})

test('an edit refused stays in its dialog, without a toast', async () => {
  stub([mo], {
    [`PATCH ${path}/${mo.id}`]: () => json(422, { code: 'validation', message: 'URL refusée.', errors: [] }),
  })
  const user = userEvent.setup()
  renderAt(`/accounts/${api.id}/webhooks`)
  await user.click(within(await row(/SMS entrants/)).getByRole('button', { name: 'Modifier l’URL' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier l’URL du webhook MO' })
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('URL refusée.')
  expect(screen.getByRole('dialog')).toBe(dialog)
  noToast()
})

test('nothing can be added before the list is read', async () => {
  let answer: (r: Response) => void = () => {}
  stub([], { [`GET ${path}`]: () => new Promise<Response>((resolve) => (answer = resolve)) })
  renderAt(`/accounts/${api.id}/webhooks`)
  const region = await block()
  expect(within(region).getByText('Chargement des webhooks…')).toBeDefined()
  expect(within(region).queryByRole('button', { name: 'Ajouter' })).toBeNull()
  answer(json(200, []))
  expect(await within(region).findByRole('button', { name: 'Ajouter' })).toBeDefined()
})
