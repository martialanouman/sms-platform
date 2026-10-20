import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { pendingChallenge } from '../session'
import { challenge, json, me, renderAt, stubFetch, violations } from '../test'

const unreachable = 'Le service ne répond pas ; réessayez.'

async function fill(email: string, password: string, user = userEvent.setup()) {
  await user.type(await screen.findByLabelText('Adresse e-mail'), email)
  await user.type(screen.getByLabelText('Mot de passe'), password)
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
}

test('a login keeps the challenge and asks for the second factor', async () => {
  const calls = stubFetch({
    'POST /api/auth/login': () => json(200, challenge),
    'GET /api/auth/me': () => json(200, { ...me, elevated: false }),
  })
  const { router } = renderAt('/login')
  const email = await screen.findByLabelText('Adresse e-mail')
  await waitFor(() => expect(document.activeElement).toBe(email))
  expect(document.title).toBe('Connexion — Tableau de bord')
  await fill('operateur@example.test', 'Correct-horse-9')
  await waitFor(() => expect(router.state.location.pathname).toBe('/mfa'))
  expect(pendingChallenge()).toBe(challenge.challenge)
  const login = calls[0]
  expect(login?.headers.get('Content-Type')).toBe('application/json')
  expect(await login?.json()).toEqual({ email: 'operateur@example.test', password: 'Correct-horse-9' })
})

test('a login without a factor leads to the enrolment', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(200, challenge),
    'GET /api/auth/me': () =>
      json(200, { ...me, elevated: false, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 0 } }),
  })
  const { router } = renderAt('/login')
  await fill('operateur@example.test', 'Correct-horse-9')
  await waitFor(() => expect(router.state.location.pathname).toBe('/enroll'))
  expect(pendingChallenge()).toBe(challenge.challenge)
})

test('a login whose session cannot be read is told, and the login stays', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(200, challenge),
    'GET /api/auth/me': () => new Response(null, { status: 502 }),
  })
  const { router } = renderAt('/login')
  await fill('operateur@example.test', 'Correct-horse-9')
  expect((await screen.findByRole('alert')).textContent).toBe(unreachable)
  expect(router.state.location.pathname).toBe('/login')
  expect(screen.getByRole('button', { name: 'Se connecter' })).toHaveProperty('disabled', false)
})

test('a refused login shows the server message, focuses it and frees the button', async () => {
  stubFetch({
    'POST /api/auth/login': () =>
      json(401, { code: 'unauthenticated', message: 'Adresse ou mot de passe refusé.' }, { 'Retry-After': '2' }),
  })
  renderAt('/login')
  await fill('operateur@example.test', 'wrong')
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe('Adresse ou mot de passe refusé.')
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Adresse e-mail').getAttribute('aria-describedby')).toBe(alert.id)
  expect(screen.getByLabelText('Mot de passe').getAttribute('aria-describedby')).toBe(alert.id)
  expect(screen.getByRole('button', { name: 'Se connecter' })).toHaveProperty('disabled', false)
})

test('a second identical refusal focuses the alert again', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(401, { code: 'unauthenticated', message: 'Adresse ou mot de passe refusé.' }),
  })
  renderAt('/login')
  const user = userEvent.setup()
  await fill('operateur@example.test', 'wrong', user)
  await screen.findByRole('alert')
  await user.type(screen.getByLabelText('Mot de passe'), 'again')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('alert')))
})

test.each([
  ['an unreachable server', () => Promise.reject(new TypeError('Failed to fetch'))],
  ['a proxy error without a body', () => new Response(null, { status: 502 })],
  ['a proxy error in HTML', () => new Response('<html>Bad Gateway</html>', { status: 502 })],
])('%s is told, and the login stays', async (_, answer) => {
  stubFetch({ 'POST /api/auth/login': answer })
  const { router } = renderAt('/login')
  await fill('operateur@example.test', 'Correct-horse-9')
  expect((await screen.findByRole('alert')).textContent).toBe(unreachable)
  expect(router.state.location.pathname).toBe('/login')
  expect(screen.getByRole('button', { name: 'Se connecter' })).toHaveProperty('disabled', false)
})

test('a rate limit disables the button for Retry-After seconds', async () => {
  stubFetch({
    'POST /api/auth/login': () =>
      json(429, { code: 'rate_limited', message: 'Trop de tentatives ; réessayez dans 2 s.' }, { 'Retry-After': '2' }),
  })
  vi.useFakeTimers({ shouldAdvanceTime: true })
  renderAt('/login')
  await fill('operateur@example.test', 'wrong', userEvent.setup({ advanceTimers: vi.advanceTimersByTime }))
  expect((await screen.findByRole('alert')).textContent).toBe('Trop de tentatives ; réessayez dans 2 s.')
  const button = screen.getByRole('button', { name: 'Se connecter' })
  await act(() => vi.advanceTimersByTimeAsync(1_500))
  expect(button).toHaveProperty('disabled', true)
  await act(() => vi.advanceTimersByTimeAsync(1_000))
  expect(button).toHaveProperty('disabled', false)
})

test('a busy server is told as such', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(503, { code: 'service_unavailable', message: 'Le service est saturé.' }),
  })
  renderAt('/login')
  await fill('operateur@example.test', 'Correct-horse-9')
  expect((await screen.findByRole('alert')).textContent).toBe('Le service est saturé.')
})

test.each([
  ['session-fermee', 'Votre session est fermée. Reconnectez-vous.'],
  ['mot-de-passe-enregistre', 'Mot de passe enregistré. Vous pouvez vous connecter.'],
])('the notice %s is shown', async (avis, text) => {
  stubFetch({})
  renderAt(`/login?avis=${avis}`)
  expect((await screen.findByRole('status')).textContent).toBe(text)
})

test('an unknown notice is not shown', async () => {
  stubFetch({})
  renderAt('/login?avis=autre')
  await screen.findByLabelText('Adresse e-mail')
  expect(screen.queryByRole('status')).toBeNull()
})

test('the login screen has no accessibility violation', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(401, { code: 'unauthenticated', message: 'Adresse ou mot de passe refusé.' }),
  })
  const { container } = renderAt('/login?avis=session-fermee')
  await screen.findByLabelText('Adresse e-mail')
  expect(await violations(container)).toEqual([])
  await fill('operateur@example.test', 'wrong')
  await screen.findByRole('alert')
  expect(await act(() => violations(container))).toEqual([])
})
