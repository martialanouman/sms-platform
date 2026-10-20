import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { act } from 'react'
import { expect, test, vi } from 'vitest'
import { pendingChallenge, rememberChallenge } from '../session'
import { challenge, empty, json, me, renderAt, stubFetch, violations } from '../test'

const refused = { code: 'unauthenticated', message: 'Second facteur refusé.' }

test('without a challenge, the verification sends back to the login', async () => {
  stubFetch({})
  const { router } = renderAt('/mfa')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'verification-interrompue' })
})

test('a code of the app elevates the session and opens the dashboard', async () => {
  rememberChallenge(challenge)
  const calls = stubFetch({ 'POST /api/auth/mfa/verify': () => empty(204), 'GET /api/auth/me': () => json(200, me) })
  const { router } = renderAt('/mfa')
  const field = await screen.findByLabelText('Code à six chiffres')
  await waitFor(() => expect(document.activeElement).toBe(field))
  expect(document.title).toBe('Second facteur — Tableau de bord')
  expect(field.getAttribute('autocomplete')).toBe('one-time-code')
  const user = userEvent.setup()
  await user.type(field, '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  expect(await calls[0]?.json()).toEqual({ challenge: challenge.challenge, method: 'totp', code: '123456' })
  expect(pendingChallenge()).toBeUndefined()
})

test('a recovery code is sent as such, and the app comes back', async () => {
  rememberChallenge(challenge)
  const calls = stubFetch({ 'POST /api/auth/mfa/verify': () => empty(204), 'GET /api/auth/me': () => json(200, me) })
  const { router } = renderAt('/mfa')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Code à six chiffres'), '12')
  await user.click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  const field = screen.getByLabelText('Code de récupération')
  expect(document.activeElement).toBe(field)
  expect(field).toHaveProperty('value', '')
  await user.click(screen.getByRole('button', { name: "Utiliser l'application" }))
  expect(document.activeElement).toBe(screen.getByLabelText('Code à six chiffres'))
  await user.click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  await user.type(screen.getByLabelText('Code de récupération'), 'ABCDE-FGHIJ')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  expect(await calls[0]?.json()).toEqual({
    challenge: challenge.challenge,
    method: 'recovery_code',
    code: 'ABCDE-FGHIJ',
  })
})

test('a refused code is told, focused, and the challenge stays', async () => {
  rememberChallenge(challenge)
  stubFetch({ 'POST /api/auth/mfa/verify': () => json(401, refused) })
  const { router, container } = renderAt('/mfa')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Code à six chiffres'), '000000')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe('Second facteur refusé.')
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Code à six chiffres').getAttribute('aria-describedby')).toBe(alert.id)
  expect(router.state.location.pathname).toBe('/mfa')
  expect(pendingChallenge()).toBe(challenge.challenge)
  expect(await violations(container)).toEqual([])
})

test('the verification has no accessibility violation', async () => {
  rememberChallenge(challenge)
  stubFetch({})
  const { container } = renderAt('/mfa')
  await screen.findByLabelText('Code à six chiffres')
  expect(await violations(container)).toEqual([])
  await userEvent.setup().click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  expect(await violations(container)).toEqual([])
})

test('a login completed by its second factor shows the dashboard', async () => {
  let elevated = false
  stubFetch({
    'POST /api/auth/login': () => json(200, challenge),
    'GET /api/auth/me': () => json(200, { ...me, elevated }),
    'POST /api/auth/mfa/verify': () => {
      elevated = true
      return empty(204)
    },
  })
  const { router } = renderAt('/login')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Adresse e-mail'), 'operateur@example.test')
  await user.type(screen.getByLabelText('Mot de passe'), 'Correct-horse-9')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  expect(await screen.findByText('Connecté en tant que Opérateur (operateur@example.test)')).toBeDefined()
  expect(router.state.location.pathname).toBe('/')
})

test('an expired challenge sends back to the login', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  rememberChallenge({ challenge: challenge.challenge, expiresAt: new Date(Date.now() + 60_000).toISOString() })
  stubFetch({})
  const { router } = renderAt('/mfa')
  await screen.findByLabelText('Code à six chiffres')
  await act(() => vi.advanceTimersByTimeAsync(59_000))
  expect(router.state.location.pathname).toBe('/mfa')
  await act(() => vi.advanceTimersByTimeAsync(2_000))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'verification-interrompue' })
  expect(pendingChallenge()).toBeUndefined()
})

test('switching the method clears a refusal', async () => {
  rememberChallenge(challenge)
  stubFetch({ 'POST /api/auth/mfa/verify': () => json(401, refused) })
  renderAt('/mfa')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Code à six chiffres'), '000000')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await screen.findByRole('alert')
  await user.click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  expect(screen.queryByRole('alert')).toBeNull()
  expect(screen.getByLabelText('Code de récupération').getAttribute('aria-describedby')).toBeNull()
})

test('a rate limit disables the verification for Retry-After seconds', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  rememberChallenge(challenge)
  stubFetch({
    'POST /api/auth/mfa/verify': () =>
      json(429, { code: 'rate_limited', message: 'Trop de tentatives.' }, { 'Retry-After': '2' }),
  })
  renderAt('/mfa')
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  await user.type(await screen.findByLabelText('Code à six chiffres'), '000000')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await screen.findByRole('alert')
  const button = screen.getByRole('button', { name: 'Vérifier' })
  await act(() => vi.advanceTimersByTimeAsync(1_500))
  expect(button).toHaveProperty('disabled', true)
  await act(() => vi.advanceTimersByTimeAsync(1_000))
  expect(button).toHaveProperty('disabled', false)
})
