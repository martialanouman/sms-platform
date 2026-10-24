import { focusManager } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { explained, json, me, noSession, once, renderAt, stubFetch, toasted, violations } from '../test'

const writer = { ...me, permissions: ['roles:read', 'roles:write'] }
const owner = {
  id: 'r-owner',
  name: 'Propriétaire',
  description: 'Tout.',
  isDefault: true,
  permissions: [
    'accounts:read',
    'accounts:write',
    'cdr:export_bulk',
    'cdr:read',
    'future:thing',
    'msisdn:reveal',
    'roles:read',
    'roles:write',
    'stream:read',
    'traffic:read',
  ],
  holders: ['operateur@example.test'],
}
const support = {
  id: 'r-support',
  name: 'Support',
  description: 'Lit les comptes.',
  isDefault: true,
  permissions: ['accounts:read', 'traffic:read'],
  holders: ['ada@example.test', 'zed@example.test'],
}
const readers = {
  id: 'r-readers',
  name: 'Lecteurs',
  description: 'Lit le trafic.',
  isDefault: false,
  permissions: ['traffic:read'],
  holders: [],
}
const held = { ...readers, id: 'r-held', name: 'Tenu', holders: ['ada@example.test', 'zed@example.test'] }

function row(name: string) {
  return screen.getByRole('rowheader', { name }).closest('tr') as HTMLTableRowElement
}

test('the rail leads to the roles for an operator who may read them', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, writer), 'GET /api/roles': () => json(200, [owner]) })
  const { router } = renderAt('/')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  await userEvent.setup().click(within(rail).getByRole('link', { name: 'Rôles' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/roles'))
})

test('the roles are listed in a card, typed, counted, with a footer that counts them', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, [owner, support, readers]),
  })
  const { container } = renderAt('/roles')
  expect(await within(await screen.findByRole('main')).findByRole('heading', { level: 1, name: 'Rôles' })).toBeDefined()
  expect(within(screen.getByRole('navigation', { name: 'Fil d’Ariane' })).getByText('Rôles')).toBeDefined()
  const table = await screen.findByRole('table', { name: 'Rôles du tableau de bord' })
  expect(table.closest('.ui-card')).not.toBeNull()
  expect(document.title).toBe('Rôles — Tableau de bord')
  expect(within(row('Support')).getByRole('button', { name: 'Support' })).toBeDefined()
  const cells = within(row('Support'))
    .getAllByRole('cell')
    .slice(0, 4)
    .map((c) => c.textContent)
  expect(cells).toEqual(['Lit les comptes.', 'Par défaut', '2', '2'])
  expect(within(row('Lecteurs')).getByText('Personnalisé').className).toBe('ui-tag')
  expect(within(row('Support')).getAllByRole('cell')[2]?.classList.contains('ui-table__cell--end')).toBe(true)
  expect(screen.queryByRole('button', { name: 'Voir les permissions' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Modifier' })).toBeNull()
  expect(screen.getByText('3 rôles')).toBeDefined()
  expect(await violations(container)).toEqual([])
})

test('a single role is counted in the singular', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, writer), 'GET /api/roles': () => json(200, [owner]) })
  renderAt('/roles')
  expect(await screen.findByText('1 rôle')).toBeDefined()
})

test('a default role opens read only, its keys still told', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, writer), 'GET /api/roles': () => json(200, [owner, support]) })
  const { container } = renderAt('/roles')
  const user = userEvent.setup()
  const opener = await screen.findByRole('button', { name: 'Support' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Support Par défaut' })
  expect(dialog.textContent).toContain('Les rôles par défaut ne se modifient pas : chaque déploiement les réécrit.')
  expect(dialog.textContent).toContain('Lit les comptes.')
  expect(within(dialog).queryByRole('textbox')).toBeNull()
  const boxes = within(dialog).getAllByRole('checkbox')
  expect(boxes.every((b) => b.getAttribute('aria-readonly') === 'true')).toBe(true)
  const read = within(dialog).getByRole('group', { name: 'Comptes' })
  expect(within(read).getByRole('checkbox', { name: 'Lire' }).getAttribute('aria-checked')).toBe('true')
  expect(explained(within(read).getByRole('checkbox', { name: 'Lire' }))).toBe('accounts:read')
  expect(within(dialog).queryByRole('button', { name: 'Enregistrer' })).toBeNull()
  expect(within(dialog).queryByRole('button', { name: 'Supprimer' })).toBeNull()
  expect(await violations(container)).toEqual([])
  await user.click(within(dialog).getAllByRole('button', { name: 'Fermer' })[1] as HTMLElement)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
})

test('a role is composed in French, grouped in the order of the mockup, keys in bubbles, the sensitive ones tagged', async () => {
  let list = [support, owner]
  let reads = 0
  const calls = stubFetch({
    'GET /api/auth/me': () => {
      reads++
      return json(200, writer)
    },
    'GET /api/roles': () => json(200, list),
    'POST /api/roles': async (r) => {
      const body = await r.clone().json()
      if (body.name === 'Support') {
        return json(409, { code: 'role_name_taken', message: 'Un rôle porte déjà ce nom.' })
      }
      list = [support, owner, readers]
      return json(201, readers)
    },
  })
  const { container } = renderAt('/roles')
  const user = userEvent.setup()
  const opener = await screen.findByRole('button', { name: 'Nouveau rôle' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau rôle' })
  expect([...dialog.querySelectorAll('fieldset > legend')].map((l) => l.textContent)).toEqual([
    'Comptes',
    'Trafic',
    'Temps réel',
    'CDR',
    'Numéros des abonnés',
    'Rôles',
    'future',
  ])
  expect(within(dialog).getAllByRole('checkbox').length).toBe(10)
  expect(
    within(within(dialog).getByRole('group', { name: 'CDR' })).getByRole('checkbox', {
      name: 'Exporter en masse sensible',
    }),
  ).toBeDefined()
  expect(within(dialog).getByRole('checkbox', { name: 'future:thing' })).toBeDefined()
  expect(
    [...dialog.querySelectorAll('.ui-tag--sensitive')].map((t) => t.closest('label')?.querySelector('[role=checkbox]')),
  ).toEqual([
    within(within(dialog).getByRole('group', { name: 'CDR' })).getByRole('checkbox', { name: /Exporter/ }),
    within(dialog).getByRole('checkbox', { name: /Révéler/ }),
  ])
  const traffic = within(within(dialog).getByRole('group', { name: 'Trafic' })).getByRole('checkbox', { name: 'Lire' })
  const stream = within(within(dialog).getByRole('group', { name: 'Temps réel' })).getByRole('checkbox', {
    name: 'Lire',
  })
  expect(stream.getAttribute('aria-readonly')).toBeNull()
  expect(explained(stream)).toBe('stream:read')
  expect(stream.getAttribute('aria-checked')).toBe('false')
  traffic.focus()
  await waitFor(() => expect(document.querySelector('.ui-tooltip')?.textContent).toBe('traffic:read'))
  expect(await violations(container)).toEqual([])
  await user.type(within(dialog).getByLabelText('Nom'), 'Support')
  await user.type(within(dialog).getByLabelText('Description'), 'Lit le trafic.')
  await user.click(traffic)
  expect(stream.getAttribute('aria-checked')).toBe('true')
  expect(stream.getAttribute('aria-readonly')).toBe('true')
  expect(explained(stream)).toBe('stream:read · compris avec la lecture du trafic')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le rôle' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Un rôle porte déjà ce nom.')
  await user.clear(within(dialog).getByLabelText('Nom'))
  await user.type(within(dialog).getByLabelText('Nom'), 'Lecteurs')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le rôle' }))
  expect(await screen.findByRole('rowheader', { name: 'Lecteurs' })).toBeDefined()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  expect(await toasted()).toEqual(['Rôle Lecteurs créé.'])
  const created = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/api/roles'))
  expect(await created[1]?.json()).toEqual({
    name: 'Lecteurs',
    description: 'Lit le trafic.',
    permissions: ['traffic:read', 'stream:read'],
  })
  expect(created[1]?.headers.get('Content-Type')).toBe('application/json')
  await waitFor(() => expect(reads).toBe(3))
})

test('unchecking the traffic leaves its stream checked, and free', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, [owner, { ...readers, permissions: ['stream:read', 'traffic:read'] }]),
    'PATCH /api/roles/r-readers': async (r) => json(200, { ...readers, ...(await r.clone().json()) }),
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Lecteurs' }))
  const dialog = await screen.findByRole('dialog', { name: 'Lecteurs Personnalisé' })
  const stream = within(within(dialog).getByRole('group', { name: 'Temps réel' })).getByRole('checkbox', {
    name: 'Lire',
  })
  expect(stream.getAttribute('aria-readonly')).toBe('true')
  await user.click(
    within(within(dialog).getByRole('group', { name: 'Trafic' })).getByRole('checkbox', { name: 'Lire' }),
  )
  expect(stream.getAttribute('aria-checked')).toBe('true')
  expect(stream.getAttribute('aria-readonly')).toBeNull()
  await user.click(stream)
  expect(stream.getAttribute('aria-checked')).toBe('false')
  await user.click(stream)
  expect(stream.getAttribute('aria-checked')).toBe('true')
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
  expect(await calls.find((c) => c.method === 'PATCH')?.json()).toEqual({
    description: 'Lit le trafic.',
    permissions: ['stream:read'],
  })
})

test('a stream checked alone survives a change to another key', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, [owner, { ...readers, permissions: ['cdr:read', 'stream:read'] }]),
    'PATCH /api/roles/r-readers': async (r) => json(200, { ...readers, ...(await r.clone().json()) }),
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Lecteurs' }))
  const dialog = await screen.findByRole('dialog', { name: 'Lecteurs Personnalisé' })
  await user.click(within(within(dialog).getByRole('group', { name: 'CDR' })).getByRole('checkbox', { name: 'Lire' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
  expect(await calls.find((c) => c.method === 'PATCH')?.json()).toEqual({
    description: 'Lit le trafic.',
    permissions: ['stream:read'],
  })
})

test('within a group, Lire comes first, then Modifier, then the particular verb', async () => {
  const catalog = {
    ...owner,
    permissions: [
      'billing:read',
      'billing:topup',
      'billing:write',
      'cdr:export_bulk',
      'cdr:read',
      'content:erase',
      'content:read',
      'content:write',
      'suppressions:delete',
      'suppressions:read',
      'suppressions:write',
    ],
  }
  stubFetch({ 'GET /api/auth/me': () => json(200, writer), 'GET /api/roles': () => json(200, [catalog]) })
  renderAt('/roles')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Nouveau rôle' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouveau rôle' })
  const order = (group: string) =>
    within(within(dialog).getByRole('group', { name: group }))
      .getAllByRole('checkbox')
      .map((c) => c.closest('label')?.textContent)
  expect(order('Facturation')).toEqual(['Lire', 'Modifier', 'Recharger, ajuster et fixer les conditions de crédit sensible'])
  expect(order('Désabonnements')).toEqual(['Lire', 'Modifier', 'Supprimer sensible'])
  expect(order('Corps des messages')).toEqual(['Lire sensible', 'Modifier sensible', 'Effacer sensible'])
  expect(order('CDR')).toEqual(['Lire', 'Exporter en masse sensible'])
})

test('a custom role is edited in its dialog, and a refusal is told there', async () => {
  let list = [owner, readers]
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, list),
    'PATCH /api/roles/r-readers': async (r) => {
      const body = await r.clone().json()
      if (body.permissions.length === 0) {
        return json(409, { code: 'self_lockout', message: 'Ce changement vous retirerait la gestion des rôles.' })
      }
      list = [owner, { ...readers, ...body }]
      return json(200, list[1])
    },
  })
  renderAt('/roles')
  const user = userEvent.setup()
  const opener = await screen.findByRole('button', { name: 'Lecteurs' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Lecteurs Personnalisé' })
  expect(within(dialog).queryByLabelText('Nom')).toBeNull()
  expect((within(dialog).getByLabelText('Description') as HTMLInputElement).value).toBe('Lit le trafic.')
  const traffic = within(within(dialog).getByRole('group', { name: 'Trafic' })).getByRole('checkbox', { name: 'Lire' })
  expect(traffic.getAttribute('aria-checked')).toBe('true')
  await user.click(traffic)
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe(
    'Ce changement vous retirerait la gestion des rôles.',
  )
  await user.click(
    within(within(dialog).getByRole('group', { name: 'Comptes' })).getByRole('checkbox', { name: 'Lire' }),
  )
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(within(row('Lecteurs')).getAllByRole('cell')[2]?.textContent).toBe('1'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  expect(await toasted()).toEqual(['Rôle Lecteurs enregistré.'])
  const patches = calls.filter((c) => c.method === 'PATCH')
  expect(await patches[1]?.json()).toEqual({ description: 'Lit le trafic.', permissions: ['accounts:read'] })
})

test('a role nobody holds is deleted from the list after a confirmation, and the focus goes to the title', async () => {
  let list = [owner, readers]
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, list),
    'DELETE /api/roles/r-readers': () => {
      list = [owner]
      return new Response(null, { status: 204 })
    },
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Lecteurs' })
  await user.click(within(row('Lecteurs')).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog', { name: 'Supprimer le rôle Lecteurs ?' })
  expect(dialog.textContent).toContain('sa suppression ne retirera aucune permission')
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Lecteurs' })).toBeNull())
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Rôles' }))
  expect(await toasted()).toEqual(['Rôle Lecteurs supprimé.'])
  expect(calls.filter((c) => c.method === 'DELETE').length).toBe(1)
})

test('a role is deleted from its dialog, which closes on success', async () => {
  let list = [owner, readers]
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, list),
    'DELETE /api/roles/r-readers': () => {
      list = [owner]
      return new Response(null, { status: 204 })
    },
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Lecteurs' }))
  const fiche = await screen.findByRole('dialog', { name: 'Lecteurs Personnalisé' })
  await user.click(within(fiche).getByRole('button', { name: 'Supprimer' }))
  const confirm = await screen.findByRole('alertdialog', { name: 'Supprimer le rôle Lecteurs ?' })
  await user.click(within(confirm).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Lecteurs' })).toBeNull())
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.queryByRole('alertdialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Rôles' }))
})

test('a role held since the list was read is refused in its dialog, and the list read again', async () => {
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => {
      reads++
      return json(200, [owner, readers])
    },
    'DELETE /api/roles/r-readers': () =>
      json(409, { code: 'role_held', message: 'Le rôle est détenu par ada@example.test (serveur).' }),
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Lecteurs' })
  await user.click(within(row('Lecteurs')).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe(
    'Le rôle est détenu par ada@example.test (serveur).',
  )
  await waitFor(() => expect(reads).toBe(2))
})

test('a default role, a held role and a reader without roles:write are refused, explained', async () => {
  let permissions = writer.permissions
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...writer, permissions }),
    'GET /api/roles': () => json(200, [owner, support, held]),
  })
  const { container } = renderAt('/roles')
  await screen.findByRole('rowheader', { name: 'Support' })
  const remove = (name: string) => within(row(name)).getByRole('button', { name: 'Supprimer' })
  expect(remove('Support').getAttribute('aria-disabled')).toBe('true')
  expect(explained(remove('Support'))).toBe(
    'Rôle par défaut : réécrit à chaque déploiement, il ne se modifie ni ne se supprime.',
  )
  expect(explained(remove('Tenu'))).toBe(
    'Détenu par ada@example.test, zed@example.test : il se retire d’abord à ces opérateurs.',
  )
  expect(await violations(container)).toEqual([])
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Tenu' }))
  const fiche = await screen.findByRole('dialog', { name: 'Tenu Personnalisé' })
  expect(explained(within(fiche).getByRole('button', { name: 'Supprimer' }))).toBe(
    'Détenu par ada@example.test, zed@example.test : il se retire d’abord à ces opérateurs.',
  )
  expect(within(fiche).getByRole('button', { name: 'Enregistrer' }).hasAttribute('aria-disabled')).toBe(false)
  await user.keyboard('{Escape}')
  permissions = ['roles:read']
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await waitFor(() =>
    expect(explained(screen.getByRole('button', { name: 'Nouveau rôle' }))).toBe(
      'La composition d’un rôle demande Rôles : modifier roles:write.',
    ),
  )
  expect(explained(remove('Tenu'))).toBe('Cette action demande Rôles : modifier roles:write.')
  await user.click(screen.getByRole('button', { name: 'Tenu' }))
  const locked = await screen.findByRole('dialog', { name: 'Tenu Personnalisé' })
  expect(
    within(locked)
      .getAllByRole('checkbox')
      .every((b) => b.getAttribute('aria-readonly') === 'true'),
  ).toBe(true)
  for (const name of ['Enregistrer', 'Supprimer']) {
    expect(explained(within(locked).getByRole('button', { name }))).toBe(
      'Cette action demande Rôles : modifier roles:write.',
    )
  }
})

test('an operator who may not read the roles is told why, without a request', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }) })
  const { container } = renderAt('/roles')
  expect(
    (await screen.findByRole('heading', { level: 2, name: 'Accès aux rôles non accordé' })).parentElement?.textContent,
  ).toContain('Cet écran demande la permission Rôles : lire roles:read')
  expect(calls.some((c) => c.url.endsWith('/api/roles'))).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('the list tells it is loading, then that it cannot be read, with a retry', async () => {
  let answer: (r: Response) => void = () => {}
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => {
      reads++
      return reads === 1 ? new Promise<Response>((resolve) => (answer = resolve)) : json(200, [owner])
    },
  })
  renderAt('/roles')
  expect(await screen.findByText('Chargement des rôles…')).toBeDefined()
  await act(async () => answer(json(403, { code: 'forbidden_scope', message: 'Refusé (serveur).' })))
  const alert = await screen.findByRole('alert')
  expect(within(alert).getByRole('heading', { level: 2, name: 'Les rôles n’ont pas pu être chargés' })).toBeDefined()
  expect(alert.textContent).toContain('Refusé (serveur).')
  await userEvent.setup().click(within(alert).getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByRole('rowheader', { name: 'Propriétaire' })).toBeDefined()
})

test('a session gone while reading the roles sends to the login', async () => {
  stubFetch({ 'GET /api/auth/me': once(() => json(200, writer)), 'GET /api/roles': noSession })
  const { router } = renderAt('/roles')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'session-closed' })
})

test('a deletion that succeeds after its dialog was closed moves nothing', async () => {
  let release: () => void = () => {}
  let list = [owner, readers]
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, list),
    'DELETE /api/roles/r-readers': async () => {
      await new Promise<void>((resolve) => (release = resolve))
      list = [owner]
      return new Response(null, { status: 204 })
    },
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Lecteurs' })
  await user.click(within(row('Lecteurs')).getByRole('button', { name: 'Supprimer' }))
  await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Supprimer' }))
  await user.keyboard('{Escape}')
  const view = screen.getByRole('button', { name: 'Propriétaire' })
  view.focus()
  await act(async () => release())
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Lecteurs' })).toBeNull())
  expect(document.activeElement).toBe(view)
  await user.click(view)
  await screen.findByRole('dialog', { name: 'Propriétaire Par défaut' })
})

test.each([
  [502, 'Le service ne répond pas ; réessayez.'],
  [429, 'Trop de requêtes (serveur).'],
])('a refusal %i of the service is not followed by a new read', async (status, message) => {
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => {
      reads++
      return json(200, [owner, readers])
    },
    'DELETE /api/roles/r-readers': () =>
      status === 429 ? json(429, { code: 'rate_limited', message }) : new Response(null, { status }),
  })
  renderAt('/roles')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Lecteurs' })
  await user.click(within(row('Lecteurs')).getByRole('button', { name: 'Supprimer' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe(message)
  expect(reads).toBe(1)
})
