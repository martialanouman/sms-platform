import { focusManager } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { explained, json, me, noSession, once, renderAt, stubFetch, violations } from '../test'

const writer = { ...me, permissions: ['roles:read', 'roles:write'] }
const owner = {
  id: 'r-owner',
  name: 'Propriétaire',
  description: 'Tout.',
  isDefault: true,
  permissions: ['accounts:read', 'accounts:write', 'roles:read', 'roles:write', 'traffic:read'],
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

test('the roles are listed with their permissions and holders counted', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, writer),
    'GET /api/roles': () => json(200, [owner, support, readers]),
  })
  const { container } = renderAt('/roles')
  expect(
    await within(await screen.findByRole('banner')).findByRole('heading', { level: 1, name: 'Rôles' }),
  ).toBeDefined()
  expect(await screen.findByRole('table', { name: 'Rôles du tableau de bord' })).toBeDefined()
  expect(document.title).toBe('Rôles — Tableau de bord')
  const cells = within(row('Support'))
    .getAllByRole('cell')
    .slice(0, 3)
    .map((c) => c.textContent)
  expect(cells).toEqual(['Lit les comptes.', '2', '2'])
  expect(within(row('Support')).getAllByRole('cell')[1]?.classList.contains('ui-table__cell--end')).toBe(true)
  expect(screen.getByRole('columnheader', { name: 'Permissions' }).classList.contains('ui-table__cell--end')).toBe(true)
  expect(await violations(container)).toEqual([])
})

test('the permissions of a role are told in French, each beside its key', async () => {
  const future = { ...support, permissions: [...support.permissions, 'accounts:export', 'future:thing'] }
  stubFetch({ 'GET /api/auth/me': () => json(200, writer), 'GET /api/roles': () => json(200, [owner, future]) })
  renderAt('/roles')
  await screen.findByRole('rowheader', { name: 'Support' })
  const user = userEvent.setup()
  const opener = within(row('Support')).getByRole('button', { name: 'Voir les permissions' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Rôle Support' })
  expect(within(dialog).getByRole('list', { name: 'Permissions du rôle Support' }).tabIndex).toBe(0)
  expect(
    within(dialog)
      .getAllByRole('listitem')
      .map((i) => i.textContent),
  ).toEqual(['Comptes — Lire accounts:read', 'Trafic — Lire traffic:read', 'Comptes accounts:export', 'future:thing'])
  await user.click(within(dialog).getAllByRole('button', { name: 'Fermer' })[1] as HTMLElement)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
})

test('a role is composed from the permissions of the owner, grouped by resource, in French', async () => {
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
  expect(
    within(dialog)
      .getAllByRole('group')
      .map((g) => g.querySelector('legend')?.textContent),
  ).toEqual(['Comptes', 'Rôles', 'Trafic'])
  expect(within(dialog).getAllByRole('checkbox').length).toBe(5)
  expect(await violations(container)).toEqual([])
  await user.type(within(dialog).getByLabelText('Nom'), 'Support')
  await user.type(within(dialog).getByLabelText('Description'), 'Lit le trafic.')
  await user.click(within(dialog).getByRole('checkbox', { name: 'Lire traffic:read' }))
  await user.click(within(dialog).getByRole('button', { name: 'Créer le rôle' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Un rôle porte déjà ce nom.')
  await user.clear(within(dialog).getByLabelText('Nom'))
  await user.type(within(dialog).getByLabelText('Nom'), 'Lecteurs')
  await user.click(within(dialog).getByRole('button', { name: 'Créer le rôle' }))
  expect(await screen.findByRole('rowheader', { name: 'Lecteurs' })).toBeDefined()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  const created = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/api/roles'))
  expect(await created[1]?.json()).toEqual({
    name: 'Lecteurs',
    description: 'Lit le trafic.',
    permissions: ['traffic:read'],
  })
  expect(created[1]?.headers.get('Content-Type')).toBe('application/json')
  await waitFor(() => expect(reads).toBe(3))
})

test('a custom role is edited, and a refusal is told in its dialog', async () => {
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
  await screen.findByRole('rowheader', { name: 'Lecteurs' })
  await user.click(within(row('Lecteurs')).getByRole('button', { name: 'Modifier' }))
  const dialog = await screen.findByRole('dialog', { name: 'Modifier Lecteurs' })
  expect(within(dialog).queryByLabelText('Nom')).toBeNull()
  const traffic = within(dialog).getByRole('checkbox', { name: 'Lire traffic:read' }) as HTMLInputElement
  expect(traffic.checked).toBe(true)
  await user.click(traffic)
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer le rôle' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe(
    'Ce changement vous retirerait la gestion des rôles.',
  )
  await user.click(within(dialog).getByRole('checkbox', { name: 'Lire accounts:read' }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer le rôle' }))
  await waitFor(() => expect(within(row('Lecteurs')).getAllByRole('cell')[1]?.textContent).toBe('1'))
  expect(screen.queryByRole('dialog')).toBeNull()
  const patches = calls.filter((c) => c.method === 'PATCH')
  expect(await patches[1]?.json()).toEqual({ description: 'Lit le trafic.', permissions: ['accounts:read'] })
})

test('a role nobody holds is deleted after a confirmation, and the focus goes to the title', async () => {
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
  const dialog = await screen.findByRole('dialog', { name: 'Supprimer le rôle Lecteurs ?' })
  expect(dialog.textContent).toContain('sa suppression ne retirera aucune permission')
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Lecteurs' })).toBeNull())
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Rôles' }))
  expect(calls.filter((c) => c.method === 'DELETE').length).toBe(1)
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
  const dialog = await screen.findByRole('dialog')
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
  const byDefault = 'Rôle par défaut : réécrit à chaque déploiement, il ne se modifie ni ne se supprime.'
  for (const name of ['Modifier', 'Supprimer']) {
    const button = within(row('Support')).getByRole('button', { name })
    expect(button.getAttribute('aria-disabled')).toBe('true')
    expect(explained(button)).toBe(byDefault)
  }
  expect(explained(within(row('Tenu')).getByRole('button', { name: 'Supprimer' }))).toBe(
    'Détenu par ada@example.test, zed@example.test : il se retire d’abord à ces opérateurs.',
  )
  expect(within(row('Tenu')).getByRole('button', { name: 'Modifier' }).hasAttribute('aria-disabled')).toBe(false)
  expect(await violations(container)).toEqual([])
  permissions = ['roles:read']
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await waitFor(() =>
    expect(explained(screen.getByRole('button', { name: 'Nouveau rôle' }))).toBe(
      'La composition d’un rôle demande roles:write.',
    ),
  )
  expect(explained(within(row('Tenu')).getByRole('button', { name: 'Modifier' }))).toBe(
    'Cette action demande roles:write.',
  )
  expect(
    within(row('Support')).getByRole('button', { name: 'Voir les permissions' }).hasAttribute('aria-disabled'),
  ).toBe(false)
})

test('an operator who may not read the roles is told why, without a request', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }) })
  const { container } = renderAt('/roles')
  expect(await screen.findByText('Il faut la permission roles:read pour lire les rôles.')).toBeDefined()
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
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Supprimer' }))
  await user.keyboard('{Escape}')
  const view = within(row('Propriétaire')).getByRole('button', { name: 'Voir les permissions' })
  view.focus()
  await act(async () => release())
  await waitFor(() => expect(screen.queryByRole('rowheader', { name: 'Lecteurs' })).toBeNull())
  expect(document.activeElement).toBe(view)
  await user.click(view)
  await screen.findByRole('dialog', { name: 'Rôle Propriétaire' })
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
  const dialog = await screen.findByRole('dialog')
  await user.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe(message)
  expect(reads).toBe(1)
})
