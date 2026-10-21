import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { json, me, renderAt, stubFetch, violations } from '../test'

test('the shell sends a request without session to the login', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }) })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'session-closed' })
})

test('the rail offers what the operator may read, and explains what is disabled', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => json(204, {}) })
  const { container } = renderAt('/')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  expect(within(rail).getByRole('link', { name: 'Mon compte' }).getAttribute('aria-current')).toBe('page')
  expect(within(rail).queryByRole('link', { name: 'Opérateurs' })).toBeNull()
  expect(within(rail).getByText('Opérateurs').getAttribute('aria-disabled')).toBe('true')
  expect(within(rail).getByText('Il faut la permission operators:read.')).toBeDefined()
  expect(screen.getByText('Opérateur')).toBeDefined()
  expect(await violations(container)).toEqual([])
})

test('the rail links to the operators for an operator who may read them', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, []),
  })
  const { router } = renderAt('/')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  await userEvent.setup().click(within(rail).getByRole('link', { name: 'Opérateurs' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/operators'))
  expect(within(rail).getByRole('link', { name: 'Opérateurs' }).getAttribute('aria-current')).toBe('page')
  expect(within(rail).getByRole('link', { name: 'Mon compte' }).getAttribute('aria-current')).toBeNull()
})

test('a session check that fails is told with a retry, without leaving', async () => {
  let failing = true
  stubFetch({
    'GET /api/auth/me': () => (failing ? new Response(null, { status: 500 }) : json(200, me)),
  })
  const { router } = renderAt('/')
  expect((await screen.findByRole('alert')).textContent).toContain('Le service ne répond pas')
  expect(router.state.location.pathname).toBe('/')
  expect(screen.queryByRole('heading', { name: 'Mon compte' })).toBeNull()
  failing = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByRole('navigation', { name: 'Navigation principale' })).toBeDefined()
})

test('the shell tells the session is loading', async () => {
  let answer: (r: Response) => void = () => {}
  stubFetch({ 'GET /api/auth/me': () => new Promise<Response>((resolve) => (answer = resolve)) })
  renderAt('/')
  expect(await screen.findByText('Chargement de la session…')).toBeDefined()
  await act(async () => answer(json(200, me)))
  expect(await screen.findByRole('heading', { name: 'Mon compte' })).toBeDefined()
  expect(screen.queryByText('Chargement de la session…')).toBeNull()
})
