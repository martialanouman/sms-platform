import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { pendingChallenge, rememberChallenge } from '../session'
import { challenge, empty, json, me, renderAt, stubFetch, violations } from '../test'

const enrollment = {
  secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  otpauthUri: 'otpauth://totp/Tableau%20de%20bord%20SMS:operateur@example.test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  recoveryCodes: Array.from({ length: 10 }, (_, i) => `CODE${i}ABCDE`),
}

async function configure(user = userEvent.setup()) {
  await user.click(await screen.findByRole('button', { name: "Configurer l'application" }))
  return user
}

test('without a challenge, the enrolment sends back to the login', async () => {
  stubFetch({})
  const { router } = renderAt('/enroll')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'verification-interrompue' })
})

test('the app is configured, proved, and only then its codes are shown', async () => {
  rememberChallenge(challenge.challenge)
  const calls = stubFetch({
    'POST /api/auth/mfa/totp/enroll': () => json(200, enrollment),
    'POST /api/auth/mfa/verify': () => empty(204),
    'GET /api/auth/me': () => json(200, me),
  })
  const { router, container } = renderAt('/enroll')
  await screen.findByRole('button', { name: "Configurer l'application" })
  expect(document.title).toBe('Second facteur — Tableau de bord')
  expect(calls).toEqual([])
  const user = await configure()
  const qr = await screen.findByRole('img', { name: "QR code de configuration de l'application" })
  expect(qr.tagName).toBe('svg')
  expect(screen.getByText(enrollment.secret)).toBeDefined()
  expect(screen.queryByText('CODE0ABCDE')).toBeNull()
  expect(await violations(container)).toEqual([])
  expect(await calls[0]?.json()).toEqual({})
  await user.type(screen.getByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  const heading = await screen.findByRole('heading', { name: 'Codes de récupération' })
  await waitFor(() => expect(document.activeElement).toBe(heading))
  expect(await calls[1]?.json()).toEqual({ challenge: challenge.challenge, method: 'totp', code: '123456' })
  const codes = within(screen.getByRole('list', { name: 'Codes de récupération' })).getAllByRole('listitem')
  expect(codes.map((c) => c.textContent)).toEqual(enrollment.recoveryCodes)
  expect(screen.queryByText(enrollment.secret)).toBeNull()
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: "J'ai enregistré ces codes" }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  expect(pendingChallenge()).toBeUndefined()
})

test('a refused code is told, and no code is shown', async () => {
  rememberChallenge(challenge.challenge)
  stubFetch({
    'POST /api/auth/mfa/totp/enroll': () => json(200, enrollment),
    'POST /api/auth/mfa/verify': () => json(401, { code: 'unauthenticated', message: 'Second facteur refusé.' }),
  })
  const { container } = renderAt('/enroll')
  const user = await configure()
  await user.type(await screen.findByLabelText('Code à six chiffres'), '000000')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe('Second facteur refusé.')
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Code à six chiffres').getAttribute('aria-describedby')).toBe(alert.id)
  expect(screen.queryByText('CODE0ABCDE')).toBeNull()
  expect(await violations(container)).toEqual([])
})

test('a refused configuration is told', async () => {
  rememberChallenge(challenge.challenge)
  stubFetch({ 'POST /api/auth/mfa/totp/enroll': () => new Response(null, { status: 502 }) })
  renderAt('/enroll')
  await configure()
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
  expect(screen.queryByRole('img')).toBeNull()
})

test('the key and the codes are copied, and a refused copy is told', async () => {
  rememberChallenge(challenge.challenge)
  stubFetch({
    'POST /api/auth/mfa/totp/enroll': () => json(200, enrollment),
    'POST /api/auth/mfa/verify': () => empty(204),
  })
  renderAt('/enroll')
  const user = await configure()
  await user.click(await screen.findByRole('button', { name: 'Copier la clé' }))
  expect(await navigator.clipboard.readText()).toBe(enrollment.secret)
  expect(screen.getByRole('status').textContent).toBe('Copié.')
  await user.type(screen.getByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await user.click(await screen.findByRole('button', { name: 'Copier les codes' }))
  expect(await navigator.clipboard.readText()).toBe(enrollment.recoveryCodes.join('\n'))
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'))
  await user.click(screen.getByRole('button', { name: 'Copier les codes' }))
  await waitFor(() =>
    expect(screen.getByRole('status').textContent).toBe(
      'Le navigateur a refusé la copie ; sélectionnez le texte et copiez-le.',
    ),
  )
})
