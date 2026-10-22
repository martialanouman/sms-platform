import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { json, me, renderAt, stubFetch, violations } from '../test'

const auditor = { ...me, permissions: ['audit:read'] }
const owner = { ...me, permissions: ['audit:read', 'operators:read'] }
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
  expect(await screen.findByRole('table', { name: 'Journal du plan de contrôle' })).toBeDefined()
  expect(document.title).toBe('Audit — Tableau de bord')
  expect(screen.getByRole('link', { name: 'Plan de contrôle' }).getAttribute('aria-current')).toBe('page')
  expect(screen.getByRole('navigation', { name: 'Navigation principale' }).textContent).toContain('Audit')
  expect(cells('create-route').slice(0, 4)).toEqual([ada, 'create-route', '/admin/routes', '403'])
  expect(calls.some((r) => ['/api/access-log', '/api/operators'].includes(new URL(r.url).pathname))).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('an operator is named when operators:read is held, and only then', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/gw/admin/audit-log': () => page([entry]),
    'GET /api/operators': () =>
      json(200, [
        {
          id: ada,
          email: 'ada@example.test',
          displayName: 'Ada',
          status: 'active',
          roles: [],
          secondFactorEnrolled: true,
          accessLink: null,
        },
      ]),
  })
  renderAt('/audit/control-plane')
  await waitFor(() => expect(cells('list-audit-log')[0]).toBe(`Ada${ada}`))
})

test('the filters and the cursor travel in the request', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, auditor),
    'GET /api/gw/admin/audit-log': (r) =>
      new URL(r.url).searchParams.get('cursor') ? page([refused]) : page([entry], 'next-1'),
  })
  renderAt('/audit/control-plane')
  const user = userEvent.setup()
  await screen.findByText('list-audit-log')
  await user.click(screen.getByRole('button', { name: 'Suivant' }))
  expect(await screen.findByText('create-route')).toBeDefined()
  expect(new URL(calls.at(-1)!.url).searchParams.get('cursor')).toBe('next-1')

  await user.type(screen.getByLabelText('Opérateur'), ada)
  await user.type(screen.getByLabelText('Depuis'), '2026-10-21T10:00')
  await user.click(screen.getByRole('button', { name: 'Filtrer' }))
  await waitFor(() => {
    const q = new URL(calls.at(-1)!.url).searchParams
    expect([q.get('operator'), q.get('from_date'), q.get('to_date'), q.get('cursor')]).toEqual([
      ada,
      new Date('2026-10-21T10:00').toISOString(),
      null,
      null,
    ])
  })
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
  expect(await screen.findByText('Il faut la permission audit:read pour lire les journaux d’audit.')).toBeDefined()
  expect(calls.map((r) => new URL(r.url).pathname)).toEqual(['/api/auth/me'])
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
  await userEvent.setup().click(screen.getByRole('button', { name: 'Suivant' }))
  await screen.findByText('Connexion')
  expect(cells('Connexion')).toEqual(['—', 'Connexion', '—', 'Échec'])
  expect(calls.some((r) => new URL(r.url).pathname.startsWith('/api/gw/'))).toBe(false)
  expect(await violations(container)).toEqual([])
})
