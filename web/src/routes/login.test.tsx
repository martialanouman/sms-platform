import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { json, me, renderAt, stubFetch, violations } from '../test'

async function fill(email: string, password: string) {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Adresse e-mail'), email)
  await user.type(screen.getByLabelText('Mot de passe'), password)
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
}

test('a login opens the dashboard and sends the credentials as JSON', async () => {
  const calls = stubFetch({
    'POST /api/auth/login': () => json(200, { challenge: 'c'.repeat(43), expiresAt: '2026-10-20T12:05:00Z' }),
    'GET /api/auth/me': () => json(200, me),
  })
  const { router } = renderAt('/login')
  const email = await screen.findByLabelText('Adresse e-mail')
  await waitFor(() => expect(document.activeElement).toBe(email))
  expect(document.title).toBe('Connexion — Tableau de bord')
  await fill('operateur@example.test', 'Correct-horse-9')
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  const login = calls[0]
  expect(login?.headers.get('Content-Type')).toBe('application/json')
  expect(await login?.json()).toEqual({ email: 'operateur@example.test', password: 'Correct-horse-9' })
})

test('a refused login shows the server message and focuses it', async () => {
  stubFetch({
    'POST /api/auth/login': () => json(401, { code: 'unauthenticated', message: 'Adresse ou mot de passe refusé.' }),
  })
  renderAt('/login')
  await fill('operateur@example.test', 'wrong')
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe('Adresse ou mot de passe refusé.')
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Mot de passe').getAttribute('aria-describedby')).toBe(alert.id)
})

test('a rate limit disables the button for Retry-After seconds', async () => {
  stubFetch({
    'POST /api/auth/login': () =>
      json(429, { code: 'rate_limited', message: 'Trop de tentatives ; réessayez dans 2 s.' }, { 'Retry-After': '2' }),
  })
  renderAt('/login')
  await fill('operateur@example.test', 'wrong')
  expect((await screen.findByRole('alert')).textContent).toBe('Trop de tentatives ; réessayez dans 2 s.')
  const button = screen.getByRole('button', { name: 'Se connecter' })
  expect(button).toHaveProperty('disabled', true)
  await waitFor(() => expect(button).toHaveProperty('disabled', false), { timeout: 3000 })
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
