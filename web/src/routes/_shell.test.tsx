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

test('the rail hides what the operator may not read, with its empty group', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => json(204, {}) })
  const { container } = renderAt('/')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  expect(within(rail).getByRole('link', { name: 'SMS Gateway' }).getAttribute('href')).toBe('/')
  expect(within(rail).queryByText('Opérateurs')).toBeNull()
  expect(within(rail).queryByText('Administration')).toBeNull()
  expect(await violations(container)).toEqual([])
})

test('the rail groups the screens the operator may read', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, []),
  })
  const { router, container } = renderAt('/')
  const rail = await screen.findByRole('navigation', { name: 'Navigation principale' })
  const group = within(rail).getByRole('list', { name: 'Administration' })
  await userEvent.setup().click(within(group).getByRole('link', { name: 'Opérateurs' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/operators'))
  expect(within(group).getByRole('link', { name: 'Opérateurs' }).getAttribute('aria-current')).toBe('page')
  expect(await violations(container)).toEqual([])
})

test('the top bar names the operator, who reaches the account through it', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  const { router } = renderAt('/')
  await userEvent.setup().click(await screen.findByRole('link', { name: 'Opérateur' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/account'))
  expect(await within(screen.getByRole('banner')).findByRole('heading', { level: 1, name: 'Mon compte' })).toBeDefined()
})

test('the skip link moves the focus to the content', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  renderAt('/')
  await userEvent.setup().click(await screen.findByRole('link', { name: 'Aller au contenu' }))
  expect(document.activeElement).toBe(screen.getByRole('main'))
})

test('the home tells the screens are coming', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  const { container } = renderAt('/')
  expect(await screen.findByRole('heading', { level: 1, name: "Le cockpit d'exploitation se construit" })).toBeDefined()
  expect(await violations(container)).toEqual([])
})

test('a session check that fails is told with a retry, without leaving', async () => {
  let failing = true
  stubFetch({
    'GET /api/auth/me': () => (failing ? new Response(null, { status: 500 }) : json(200, me)),
  })
  const { router } = renderAt('/')
  const alert = await screen.findByRole('alert')
  expect(within(alert).getByRole('heading', { level: 1, name: 'Impossible de vérifier la session' })).toBeDefined()
  expect(alert.textContent).toContain('Le service ne répond pas')
  expect(router.state.location.pathname).toBe('/')
  expect(screen.queryByRole('main')?.textContent).not.toContain('cockpit')
  failing = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByRole('navigation', { name: 'Navigation principale' })).toBeDefined()
})

test('the shell tells the session is loading', async () => {
  let answer: (r: Response) => void = () => {}
  stubFetch({ 'GET /api/auth/me': () => new Promise<Response>((resolve) => (answer = resolve)) })
  renderAt('/')
  expect(await screen.findByText('Ouverture de la session')).toBeDefined()
  await act(async () => answer(json(200, me)))
  expect(await screen.findByRole('heading', { name: "Le cockpit d'exploitation se construit" })).toBeDefined()
  expect(screen.queryByText('Ouverture de la session')).toBeNull()
})

test('a session not elevated is sent back to the login from the home', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, elevated: false }) })
  const { router } = renderAt('/')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'verification-interrupted' })
})
