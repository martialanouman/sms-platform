import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
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
  rememberChallenge(challenge.challenge)
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
  rememberChallenge(challenge.challenge)
  const calls = stubFetch({ 'POST /api/auth/mfa/verify': () => empty(204), 'GET /api/auth/me': () => json(200, me) })
  const { router } = renderAt('/mfa')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Utiliser un code de récupération' }))
  const field = screen.getByLabelText('Code de récupération')
  expect(document.activeElement).toBe(field)
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
  rememberChallenge(challenge.challenge)
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
  rememberChallenge(challenge.challenge)
  stubFetch({})
  const { container } = renderAt('/mfa')
  await screen.findByLabelText('Code à six chiffres')
  expect(await violations(container)).toEqual([])
  await userEvent.setup().click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  expect(await violations(container)).toEqual([])
})
