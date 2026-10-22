import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { json, me, renderAt, stubFetch, violations } from '../test'

const auditor = { ...me, permissions: ['audit:read'] }
const ada = '01a159b0-0000-7000-8000-00000000000a'
const entry = {
  id: '01a16026-0000-7000-8000-000000000001',
  operator: ada,
  operation_id: 'list-audit-log',
  target: '/admin/audit-log',
  request_id: 'r-read',
  status: 200,
  outcome: null,
  at: '2026-10-21T22:18:27Z',
}
const refused = {
  ...entry,
  id: '01a16026-0000-7000-8000-000000000002',
  operation_id: 'create-route',
  target: '/admin/routes',
  request_id: 'r-write',
  status: 403,
}
const page = (data: unknown[], next: string | null = null) => json(200, { data, next_cursor: next })

function cells(text: string) {
  return within(screen.getByText(text).closest('tr') as HTMLTableRowElement)
    .getAllByRole('cell')
    .map((c) => c.textContent)
}

test('the rail offers Audit to whoever holds audit:read only', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, []),
  })
  renderAt('/operators')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  await within(rail).findByRole('link', { name: 'Opérateurs' })
  expect(within(rail).queryByRole('link', { name: 'Audit' })).toBeNull()
})

test('the control plane tab lists the gateway journal, newest first', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () => page([refused, entry]),
  })
  const { container } = renderAt('/audit')
  expect(await screen.findByRole('table', { name: 'Journal d’administration de la passerelle' })).toBeDefined()
  expect(document.title).toBe('Audit — Tableau de bord')
  expect(screen.getByRole('link', { name: 'Administration de la passerelle' }).getAttribute('aria-current')).toBe(
    'page',
  )
  expect(screen.getByRole('navigation', { name: 'Navigation principale' }).textContent).toContain('Audit')
  expect(cells('create-route').slice(0, 4)).toEqual([ada, 'create-route', '/admin/routes', '403'])
  expect(calls.some((r) => ['/api/access-log', '/api/operators'].includes(new URL(r.url).pathname))).toBe(false)
  expect(await violations(container)).toEqual([])
})

const directory = () =>
  json(200, [
    { id: ada, email: 'ada@example.test', displayName: 'Ada', status: 'active' },
    { id: 'gone', email: 'bob@example.test', displayName: 'Bob', status: 'disabled' },
  ])

test('an operator is named from the audit directory, with audit:read alone', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () => page([entry]),
    'GET /api/audit/operators': directory,
  })
  renderAt('/audit/control-plane')
  await waitFor(() => expect(cells('list-audit-log')[0]).toBe('Adaada@example.test'))
})

async function choose(user: ReturnType<typeof userEvent.setup>, typed: string, name: string) {
  const box = await screen.findByRole('combobox', { name: 'Opérateur' })
  await user.click(box)
  await user.type(box, typed)
  await user.click(await screen.findByRole('option', { name: new RegExp(`^${name}`) }))
}

test('the operator is chosen among every operator, by name or email, and sent by its id', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () => page([entry]),
    'GET /api/audit/operators': directory,
  })
  const { container } = renderAt('/audit/control-plane')
  const user = userEvent.setup()
  await waitFor(() => expect(cells('list-audit-log')[0]).toBe('Adaada@example.test'))
  const box = screen.getByRole('combobox', { name: 'Opérateur' })
  await user.click(box)
  expect((await screen.findAllByRole('option')).map((o) => o.textContent)).toEqual([
    'Adaada@example.test',
    'Bob (désactivé)bob@example.test',
  ])
  await user.type(box, 'BOB@EX')
  await waitFor(() =>
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['Bob (désactivé)bob@example.test']),
  )
  await user.clear(box)
  await user.type(box, 'ad')
  await waitFor(() => expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['Adaada@example.test']))
  await user.click(screen.getByRole('option', { name: /^Ada/ }))
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Filtrer' }))
  await waitFor(() => expect(new URL(calls.at(-1)?.url ?? '').searchParams.get('operator')).toBe(ada))
})

test('an operator carried by the address shows by its name once the directory is read', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/access-log': () => json(200, { data: [], nextCursor: null }),
    'GET /api/audit/operators': directory,
  })
  renderAt(`/audit/access?operator=${ada}`)
  await waitFor(() =>
    expect((screen.getByRole('combobox', { name: 'Opérateur' }) as HTMLInputElement).value).toBe('Ada'),
  )
})

const local = (day: string) => new Date(`${day}T00:00`).toISOString()
const today = new Date().toLocaleDateString('sv-SE')
const tomorrow = new Date(Date.now() + 86_400_000).toLocaleDateString('sv-SE')

test('the period defaults to today, from its start to its end', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () => page([entry]),
  })
  renderAt('/audit/control-plane')
  await screen.findByText('list-audit-log')
  const q = new URL(calls.at(-1)?.url ?? '').searchParams
  expect([q.get('from_date'), q.get('to_date')]).toEqual([local(today), local(tomorrow)])
  expect((screen.getByLabelText('Depuis') as HTMLInputElement).value).toBe(today)
  expect((screen.getByLabelText('Jusqu’à') as HTMLInputElement).value).toBe(today)
  expect((screen.getByLabelText('Depuis') as HTMLInputElement).type).toBe('date')
})

test('the filters and the cursor travel in the request', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': (r) =>
      new URL(r.url).searchParams.get('cursor') ? page([refused]) : page([entry], 'next-1'),
    'GET /api/audit/operators': directory,
  })
  renderAt('/audit/control-plane')
  const user = userEvent.setup()
  await screen.findByText('list-audit-log')
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  expect(await screen.findByText('create-route')).toBeDefined()
  expect(new URL(calls.at(-1)?.url ?? '').searchParams.get('cursor')).toBe('next-1')

  await choose(user, 'ada', 'Ada')
  await user.clear(screen.getByLabelText('Depuis'))
  await user.type(screen.getByLabelText('Depuis'), '2026-10-20')
  await user.clear(screen.getByLabelText('Jusqu’à'))
  await user.type(screen.getByLabelText('Jusqu’à'), '2026-10-21')
  await user.click(screen.getByRole('button', { name: 'Filtrer' }))
  await waitFor(() => {
    const q = new URL(calls.at(-1)?.url ?? '').searchParams
    expect([q.get('operator'), q.get('from_date'), q.get('to_date'), q.get('cursor')]).toEqual([
      ada,
      local('2026-10-20'),
      local('2026-10-22'),
      null,
    ])
  })
})

test('Previous walks back the pages one at a time, and shows only when there is one behind', async () => {
  const third = { ...entry, id: '01a16026-0000-7000-8000-000000000003', operation_id: 'delete-route' }
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': (r) =>
      ({
        'next-1': page([refused], 'next-2'),
        'next-2': page([third]),
      })[new URL(r.url).searchParams.get('cursor') ?? ''] ?? page([entry], 'next-1'),
    'GET /api/audit/operators': directory,
  })
  renderAt('/audit/control-plane')
  const user = userEvent.setup()
  const cursor = () => new URL(calls.at(-1)?.url ?? '').searchParams.get('cursor')
  await screen.findByText('list-audit-log')
  expect(screen.queryByRole('button', { name: 'Précédent' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  await screen.findByText('create-route')
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  await screen.findByText('delete-route')
  expect(screen.queryByRole('button', { name: 'Suivant' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Précédent' }))
  await screen.findByText('create-route')
  expect(cursor()).toBe('next-1')
  await user.click(screen.getByRole('button', { name: 'Précédent' }))
  await screen.findByText('list-audit-log')
  expect(cursor()).toBeNull()
  expect(screen.queryByRole('button', { name: 'Précédent' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  await screen.findByText('create-route')
  await user.click(screen.getByRole('button', { name: 'Filtrer' }))
  await screen.findByText('list-audit-log')
  expect(screen.queryByRole('button', { name: 'Précédent' })).toBeNull()
})

test('the changes of a request show before and after, the changed columns marked', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () => page([refused, entry]),
    'GET /api/gw/admin/audit-log/r-write/changes': () =>
      json(200, {
        data: [
          {
            actor: ada,
            table: 'routes',
            pk: '{"id": 7}',
            op: 'update',
            before: { name: 'a', priority: 1 },
            after: { name: 'b', priority: 1 },
            at: entry.at,
          },
        ],
        next_cursor: null,
      }),
    'GET /api/gw/admin/audit-log/r-read/changes': () => json(200, { data: [], next_cursor: null }),
  })
  renderAt('/audit/control-plane')
  const user = userEvent.setup()
  await screen.findByText('create-route')
  await user.click(
    within(screen.getByText('create-route').closest('tr') as HTMLElement).getByRole('button', {
      name: 'Modifications',
    }),
  )
  const dialog = await screen.findByRole('dialog', { name: 'Modifications de la requête r-write' })
  expect(await within(dialog).findByText('routes')).toBeDefined()
  const changed = within(dialog).getByText('name').closest('div') as HTMLElement
  expect(changed.className).toContain('changed')
  expect(changed.textContent).toContain('"a"')
  expect(changed.textContent).toContain('"b"')
  expect((within(dialog).getByText('priority').closest('div') as HTMLElement).className).not.toContain('changed')
  await user.click(within(dialog).getByRole('button', { name: 'Fermer' }))

  await user.click(
    within(screen.getByText('list-audit-log').closest('tr') as HTMLElement).getByRole('button', {
      name: 'Modifications',
    }),
  )
  const none = await screen.findByRole('dialog', { name: 'Modifications de la requête r-read' })
  expect(await within(none).findByText('Aucune modification enregistrée')).toBeDefined()
})

test('a refusal of the gateway is told with its message, an outage with a retry', async () => {
  let down = true
  stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () =>
      down ? json(503, { code: 'service_unavailable', message: 'La passerelle ne répond pas.' }) : page([entry]),
  })
  renderAt('/audit/control-plane')
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toContain('Le service ne répond pas ; réessayez.')
  down = false
  await userEvent.setup().click(within(alert).getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByText('list-audit-log')).toBeDefined()
})

test('a scope the gateway refuses is told in its words', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': () =>
      json(403, { code: 'forbidden_scope', message: 'the operator token lacks audit:read' }),
  })
  renderAt('/audit/control-plane')
  expect((await screen.findByRole('alert')).textContent).toContain('the operator token lacks audit:read')
})

test('without audit:read the screen says why and asks nothing', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }) })
  renderAt('/audit/access')
  const title = await screen.findByRole('heading', { level: 2, name: 'Accès aux journaux d’audit non accordé' })
  const state = title.parentElement as HTMLElement
  expect(state.textContent).toBe(
    'Accès aux journaux d’audit non accordé' +
      'Ces journaux retracent les appels à l’administration de la passerelle et les accès au tableau de bord. ' +
      'Cet écran demande la permission Journal d’audit : lire audit:read, qu’aucun rôle de ce compte n’accorde. ' +
      'Un opérateur habilité à modifier les rôles des opérateurs peut ajouter le rôle « Audit » à ce compte.',
  )
  expect(within(state).getByText('audit:read').className).toBe('role-keys__key')
  expect(state.querySelector('.ui-empty__icon')).not.toBeNull()
  expect(calls.map((r) => new URL(r.url).pathname)).toEqual(['/api/auth/me'])
})

test('the access tab is filtered by operator and period like the other', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/access-log': () => json(200, { data: [], nextCursor: null }),
    'GET /api/audit/operators': directory,
  })
  renderAt('/audit/access')
  const user = userEvent.setup()
  await screen.findByText('Aucune entrée')
  let q = new URL(calls.at(-1)?.url ?? '').searchParams
  expect([q.get('actor'), q.get('fromDate'), q.get('toDate')]).toEqual([null, local(today), local(tomorrow)])
  await choose(user, 'ada', 'Ada')
  await user.clear(screen.getByLabelText('Depuis'))
  await user.type(screen.getByLabelText('Depuis'), '2026-10-20')
  await user.click(screen.getByRole('button', { name: 'Filtrer' }))
  await waitFor(() => {
    q = new URL(calls.at(-1)?.url ?? '').searchParams
    expect([q.get('actor'), q.get('fromDate'), q.get('toDate')]).toEqual([ada, local('2026-10-20'), local(tomorrow)])
  })
})

test('the access tab lists the dashboard journal in French, on its own', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/access-log': (r) =>
      new URL(r.url).searchParams.get('cursor') === 'n2'
        ? json(200, {
            data: [{ id: 'a2', at: entry.at, actor: null, event: 'login', target: null, outcome: 'failure' }],
            nextCursor: null,
          })
        : json(200, {
            data: [
              { id: 'a1', at: entry.at, actor: ada, event: 'personal_token_issued', target: null, outcome: 'success' },
            ],
            nextCursor: 'n2',
          }),
  })
  const { container } = renderAt('/audit/access')
  expect(await screen.findByRole('table', { name: 'Journal d’accès au tableau de bord' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'Accès au dashboard' }).getAttribute('aria-current')).toBe('page')
  expect(cells('Jeton personnel délivré')).toEqual([ada, 'Jeton personnel délivré', '—', 'Réussi'])
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  await screen.findByText('Connexion')
  expect(cells('Connexion')).toEqual(['—', 'Connexion', '—', 'Échec'])
  await user.click(screen.getByRole('button', { name: 'Précédent' }))
  await screen.findByText('Jeton personnel délivré')
  expect(new URL(calls.at(-1)?.url ?? '').searchParams.get('cursor')).toBeNull()
  expect(calls.some((r) => new URL(r.url).pathname.startsWith('/api/gw/'))).toBe(false)
  expect(await violations(container)).toEqual([])
})
