import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { empty, json, me, renderAt, stubFetch, violations } from '../test'

test('the dashboard shows who is signed in', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  const { container } = renderAt('/')
  expect(await screen.findByText('Connecté en tant que Opérateur (operateur@example.test)')).toBeDefined()
  expect(document.title).toBe('Tableau de bord')
  expect(await violations(container)).toEqual([])
})

test('a session gone sends to the login with a notice', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }) })
  const { router } = renderAt('/')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'session-fermee' })
})

test('a logout is sent as JSON and returns to the login', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => empty(204),
  })
  const { router } = renderAt('/')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  const logout = calls.find((c) => c.url.endsWith('/api/auth/logout'))
  expect(logout?.headers.get('Content-Type')).toBe('application/json')
})

test('gestures report activity at most once a minute', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => empty(204) })
  renderAt('/')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  const activity = () => calls.filter((c) => c.url.endsWith('/api/auth/activity')).length
  fireEvent.keyDown(document)
  fireEvent.pointerDown(document)
  await waitFor(() => expect(activity()).toBe(1))
  await act(() => vi.advanceTimersByTimeAsync(59_000))
  fireEvent.pointerDown(document)
  expect(activity()).toBe(1)
  await act(() => vi.advanceTimersByTimeAsync(2_000))
  fireEvent.pointerDown(document)
  await waitFor(() => expect(activity()).toBe(2))
  expect(calls.find((c) => c.url.endsWith('/api/auth/activity'))?.headers.get('Content-Type')).toBe('application/json')
})

test('without a gesture for the idle timeout, the dashboard returns to the login without a request', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, idleTimeoutSeconds: 120 }),
    'POST /api/auth/activity': () => empty(204),
  })
  const { router } = renderAt('/')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  await act(() => vi.advanceTimersByTimeAsync(100_000))
  fireEvent.keyDown(document)
  await act(() => vi.advanceTimersByTimeAsync(100_000))
  expect(router.state.location.pathname).toBe('/')
  const before = calls.length
  await act(() => vi.advanceTimersByTimeAsync(21_000))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'session-fermee' })
  expect(calls.length).toBe(before)
})

test('an activity refused by the server sends to the login', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }),
  })
  const { router } = renderAt('/')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  fireEvent.keyDown(document)
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})
