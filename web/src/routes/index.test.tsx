import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { challenge, empty, json, me, renderAt, stubFetch, violations } from '../test'

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

const unreachable = 'Le service ne répond pas ; réessayez.'

test('a session check that fails otherwise is told', async () => {
  stubFetch({ 'GET /api/auth/me': () => new Response(null, { status: 500 }) })
  const { router } = renderAt('/')
  expect((await screen.findByRole('alert')).textContent).toBe(unreachable)
  expect(router.state.location.pathname).toBe('/')
})

test('a logout is sent as JSON and returns to the login', async () => {
  let release: () => void = () => {}
  const loggedOut = new Promise<void>((resolve) => {
    release = resolve
  })
  let served = false
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': async () => {
      await loggedOut
      served = true
      return json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' })
    },
    'POST /api/auth/logout': () => {
      release()
      return empty(204)
    },
  })
  const { router } = renderAt('/')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  await waitFor(() => expect(served).toBe(true))
  await act(async () => {})
  expect(router.state.location.search).toEqual({})
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
  await act(() => vi.advanceTimersByTimeAsync(50_000))
  fireEvent.pointerDown(document)
  expect(activity()).toBe(1)
  await act(() => vi.advanceTimersByTimeAsync(15_000))
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
  await act(() => vi.advanceTimersByTimeAsync(60_000))
  fireEvent.keyDown(document)
  await act(() => vi.advanceTimersByTimeAsync(90_000))
  expect(router.state.location.pathname).toBe('/')
  const before = calls.length
  await act(() => vi.advanceTimersByTimeAsync(40_000))
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

test('a logout that fails is told, and the session stays watched', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, idleTimeoutSeconds: 120 }),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => new Response(null, { status: 502 }),
  })
  const { router } = renderAt('/')
  await userEvent
    .setup({ advanceTimers: vi.advanceTimersByTime })
    .click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  expect((await screen.findByRole('alert')).textContent).toBe(unreachable)
  expect(router.state.location.pathname).toBe('/')
  await act(() => vi.advanceTimersByTimeAsync(130_000))
  await waitFor(() => expect(router.state.location.search).toEqual({ avis: 'session-fermee' }))
})

test('without an idle timeout, no timer sends to the login', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, idleTimeoutSeconds: null }) })
  const { router } = renderAt('/')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  await act(() => vi.advanceTimersByTimeAsync(3_600_000))
  expect(router.state.location.pathname).toBe('/')
})

test('after a logout, the next login shows the new operator only', async () => {
  let current = me
  let gated = false
  let open: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    open = resolve
  })
  stubFetch({
    'GET /api/auth/me': async () => {
      if (gated) {
        await gate
      }
      return json(200, current)
    },
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => empty(204),
    'POST /api/auth/login': () => json(200, challenge),
    'POST /api/auth/mfa/verify': () => empty(204),
  })
  const { router } = renderAt('/')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  current = { ...me, operator: { ...me.operator, email: 'autre@example.test', displayName: 'Autre' } }
  await user.type(await screen.findByLabelText('Adresse e-mail'), 'autre@example.test')
  await user.type(screen.getByLabelText('Mot de passe'), 'Correct-horse-9')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  gated = true
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  await act(async () => {})
  expect(screen.queryByText(/Opérateur/)).toBeNull()
  open()
  expect(await screen.findByText('Connecté en tant que Autre (autre@example.test)')).toBeDefined()
})

test('a session not elevated is sent back to the login', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, elevated: false }) })
  const { router } = renderAt('/')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'verification-interrompue' })
})

const replacement = {
  secret: 'KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU',
  otpauthUri: 'otpauth://totp/Tableau%20de%20bord%20SMS:operateur@example.test?secret=KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU',
  recoveryCodes: Array.from({ length: 10 }, (_, i) => `NEW${i}ABCDEF`),
}

test('the app is replaced after a proof of the current one', async () => {
  let current = { ...me, secondFactors: { ...me.secondFactors, recoveryCodesRemaining: 3 } }
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, current),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/mfa/totp/enroll': () => json(200, replacement),
    'POST /api/auth/mfa/totp/confirm': () => {
      current = me
      return empty(204)
    },
  })
  const { container } = renderAt('/')
  const user = userEvent.setup()
  expect(await screen.findByText('3 codes de récupération restants.')).toBeDefined()
  await user.click(screen.getByRole('button', { name: "Remplacer l'application" }))
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  await user.type(screen.getByLabelText('Code de récupération'), 'OLDCODE123')
  await user.click(screen.getByRole('button', { name: 'Continuer' }))
  await screen.findByRole('img', { name: "QR code de configuration de l'application" })
  const sent = (path: string) => calls.find((c) => c.url.endsWith(path))
  expect(await sent('/api/auth/mfa/totp/enroll')?.json()).toEqual({ method: 'recovery_code', code: 'OLDCODE123' })
  await user.type(screen.getByLabelText('Code à six chiffres'), '654321')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await screen.findByRole('heading', { name: 'Codes de récupération' })
  expect(await sent('/api/auth/mfa/totp/confirm')?.json()).toEqual({ code: '654321' })
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: "J'ai enregistré ces codes" }))
  expect(await screen.findByText('10 codes de récupération restants.')).toBeDefined()
  expect(screen.queryByText('NEW0ABCDEF')).toBeNull()
})

test('a replacement can be abandoned before its proof', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => empty(204) })
  renderAt('/')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: "Remplacer l'application" }))
  await user.click(screen.getByRole('button', { name: 'Annuler' }))
  expect(screen.getByRole('button', { name: "Remplacer l'application" })).toBeDefined()
  expect(calls.filter((c) => c.method === 'POST' && !c.url.endsWith('/activity'))).toEqual([])
})

test('a refused proof is told, and the current app stays', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/mfa/totp/enroll': () => json(409, { code: 'mfa_replacement_refused', message: 'Preuve refusée.' }),
  })
  renderAt('/')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: "Remplacer l'application" }))
  await user.type(screen.getByLabelText('Code à six chiffres'), '111111')
  await user.click(screen.getByRole('button', { name: 'Continuer' }))
  expect((await screen.findByRole('alert')).textContent).toBe('Preuve refusée.')
  expect(screen.queryByRole('img')).toBeNull()
})

test('a refused new code is told, and no code is shown', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/mfa/totp/enroll': () => json(200, replacement),
    'POST /api/auth/mfa/totp/confirm': () => json(422, { code: 'mfa_code_refused', message: 'Code refusé.' }),
  })
  renderAt('/')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: "Remplacer l'application" }))
  await user.type(screen.getByLabelText('Code à six chiffres'), '111111')
  await user.click(screen.getByRole('button', { name: 'Continuer' }))
  await user.type(await screen.findByLabelText('Code à six chiffres'), '222222')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  expect((await screen.findByRole('alert')).textContent).toBe('Code refusé.')
  expect(screen.queryByRole('heading', { name: 'Codes de récupération' })).toBeNull()
})

test('an account without an app is offered no replacement', async () => {
  stubFetch({
    'GET /api/auth/me': () =>
      json(200, { ...me, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 1 } }),
  })
  renderAt('/')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  expect(screen.queryByRole('button', { name: "Remplacer l'application" })).toBeNull()
})
