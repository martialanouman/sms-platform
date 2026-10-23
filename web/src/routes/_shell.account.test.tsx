import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'
import { challenge, empty, json, me, noSession, once, renderAt, stubFetch, stubPasskeys, violations } from '../test'

test('the dashboard shows who is signed in', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  const { container } = renderAt('/account')
  expect(await screen.findByText('Connecté en tant que Opérateur (operateur@example.test)')).toBeDefined()
  expect(document.title).toBe('Mon compte — Tableau de bord')
  expect(await violations(container)).toEqual([])
})

const unreachable = 'Le service ne répond pas ; réessayez.'

test('a logout is sent as JSON and returns to the login', async () => {
  let release: () => void = () => {}
  const loggedOut = new Promise<void>((resolve) => {
    release = resolve
  })
  let served = false
  const calls = stubFetch({
    'GET /api/auth/me': once(() => json(200, me)),
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
  const { router } = renderAt('/account')
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
  renderAt('/account')
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

test('without a gesture for the idle timeout, the dashboard returns to the login without reporting activity', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  const calls = stubFetch({
    'GET /api/auth/me': once(() => json(200, { ...me, idleTimeoutSeconds: 120 })),
    'POST /api/auth/activity': () => empty(204),
  })
  const { router } = renderAt('/account')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  await act(() => vi.advanceTimersByTimeAsync(60_000))
  fireEvent.keyDown(document)
  await act(() => vi.advanceTimersByTimeAsync(90_000))
  expect(router.state.location.pathname).toBe('/account')
  const activity = () => calls.filter((c) => c.url.endsWith('/api/auth/activity')).length
  const before = activity()
  await act(() => vi.advanceTimersByTimeAsync(40_000))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'session-closed' })
  expect(activity()).toBe(before)
})

test('an activity refused by the server sends to the login', async () => {
  stubFetch({
    'GET /api/auth/me': once(() => json(200, me)),
    'POST /api/auth/activity': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }),
  })
  const { router } = renderAt('/account')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  fireEvent.keyDown(document)
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
})

test('a logout that fails is told, and the session stays watched', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  stubFetch({
    'GET /api/auth/me': once(() => json(200, { ...me, idleTimeoutSeconds: 120 })),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => new Response(null, { status: 502 }),
  })
  const { router } = renderAt('/account')
  await userEvent
    .setup({ advanceTimers: vi.advanceTimersByTime })
    .click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  expect((await screen.findByRole('alert')).textContent).toBe(unreachable)
  expect(router.state.location.pathname).toBe('/account')
  await act(() => vi.advanceTimersByTimeAsync(130_000))
  await waitFor(() => expect(router.state.location.search).toEqual({ notice: 'session-closed' }))
})

test('without an idle timeout, no timer sends to the login', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, idleTimeoutSeconds: null }) })
  const { router } = renderAt('/account')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  await act(() => vi.advanceTimersByTimeAsync(3_600_000))
  expect(router.state.location.pathname).toBe('/account')
})

test('after a logout, the next login shows the new operator only', async () => {
  let current = me
  let signedIn = true
  stubFetch({
    'GET /api/auth/me': () => (signedIn ? json(200, current) : noSession()),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => {
      signedIn = false
      return empty(204)
    },
    'POST /api/auth/login': () => {
      signedIn = true
      return json(200, challenge)
    },
    'POST /api/auth/mfa/verify': () => empty(204),
  })
  renderAt('/account')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Se déconnecter' }))
  current = { ...me, operator: { ...me.operator, email: 'autre@example.test', displayName: 'Autre' } }
  await user.type(await screen.findByLabelText('Adresse e-mail'), 'autre@example.test')
  await user.type(screen.getByLabelText('Mot de passe'), 'Correct-horse-9')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await user.click(await screen.findByRole('link', { name: 'Autre' }))
  expect(await screen.findByText('Connecté en tant que Autre (autre@example.test)')).toBeDefined()
  expect(screen.queryByText(/operateur@example\.test/)).toBeNull()
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
  const { container } = renderAt('/account')
  const user = userEvent.setup()
  expect(await screen.findByText('3 codes de récupération restants.')).toBeDefined()
  await user.click(screen.getByRole('button', { name: "Remplacer l'application" }))
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Utiliser un code de récupération' }))
  await user.type(screen.getByLabelText('Code de récupération'), 'OLDCODE123')
  await user.click(screen.getByRole('button', { name: 'Continuer' }))
  await screen.findByRole('img', { name: "QR code de configuration de l'application" })
  expect(document.activeElement?.textContent).toMatch(/^Scannez ce QR code/)
  const sent = (path: string) => calls.find((c) => c.url.endsWith(path))
  expect(await sent('/api/auth/mfa/totp/enroll')?.json()).toEqual({ method: 'recovery_code', code: 'OLDCODE123' })
  await user.type(screen.getByLabelText('Code à six chiffres'), '654321')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await screen.findByRole('heading', { name: 'Codes de récupération', level: 3 })
  expect(await sent('/api/auth/mfa/totp/confirm')?.json()).toEqual({ code: '654321' })
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: "J'ai enregistré ces codes" }))
  expect(await screen.findByText('10 codes de récupération restants.')).toBeDefined()
  expect(screen.queryByText('NEW0ABCDEF')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: "Application d'authentification" }))
})

test('a replacement can be abandoned before its proof', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => empty(204) })
  renderAt('/account')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: "Remplacer l'application" }))
  await user.click(screen.getByRole('button', { name: 'Annuler' }))
  expect(screen.getByRole('button', { name: "Remplacer l'application" })).toBeDefined()
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: "Application d'authentification" }))
  expect(calls.filter((c) => c.method === 'POST' && !c.url.endsWith('/activity'))).toEqual([])
})

test('a refused proof is told, and the current app stays', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, me),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/mfa/totp/enroll': () => json(409, { code: 'mfa_replacement_refused', message: 'Preuve refusée.' }),
  })
  renderAt('/account')
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
  renderAt('/account')
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
  renderAt('/account')
  await screen.findByRole('button', { name: 'Se déconnecter' })
  expect(screen.queryByRole('button', { name: "Remplacer l'application" })).toBeNull()
})

const keyed = { ...me, secondFactors: { totp: true, recoveryCodesRemaining: 10, passkeys: 2 } }
const keys = [
  { id: 'k1', name: 'Clé USB', createdAt: '2026-10-20T10:00:00Z' },
  { id: 'k2', name: 'Téléphone', createdAt: '2026-10-20T11:00:00Z' },
]

test('the passkeys are listed, added under a name and removed', async () => {
  let held = keys
  const device = stubPasskeys()
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, keyed),
    'POST /api/auth/activity': () => empty(204),
    'GET /api/auth/mfa/webauthn/passkeys': () => json(200, held),
    'POST /api/auth/mfa/webauthn/register/begin': () => json(200, { publicKey: { challenge: 'Y2hhbGxlbmdl' } }),
    'POST /api/auth/mfa/webauthn/register/finish': () => {
      held = [...keys, { id: 'k3', name: 'Portable', createdAt: '2026-10-20T12:00:00Z' }]
      return json(200, { id: 'k3' })
    },
    'DELETE /api/auth/mfa/webauthn/passkeys/k1': () => {
      held = held.filter((k) => k.id !== 'k1')
      return empty(204)
    },
  })
  const { container } = renderAt('/account')
  const user = userEvent.setup()
  await screen.findByRole('region', { name: "Clés d'accès" })
  const list = within(screen.getByRole('region', { name: "Clés d'accès" })).getByRole('list')
  expect((await within(list).findAllByRole('listitem')).map((i) => i.firstChild?.textContent)).toEqual([
    'Clé USB',
    'Téléphone',
  ])
  expect(await violations(container)).toEqual([])
  await user.type(screen.getByLabelText('Nom de la clé'), 'Portable')
  await user.click(screen.getByRole('button', { name: "Ajouter une clé d'accès" }))
  expect(await screen.findByText('Portable')).toBeDefined()
  expect(device.create).toHaveBeenCalledTimes(1)
  expect(await calls.find((c) => c.url.endsWith('/register/finish'))?.json()).toEqual({
    name: 'Portable',
    attestation: { id: 'attested' },
  })
  await user.click(screen.getByRole('button', { name: 'Retirer Clé USB' }))
  await waitFor(() => expect(screen.queryByText('Clé USB')).toBeNull())
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: "Clés d'accès" }))
  expect(screen.getByLabelText('Nom de la clé')).toHaveProperty('value', '')
})

test('removing the last factor is refused and told', async () => {
  stubFetch({
    'GET /api/auth/me': () =>
      json(200, { ...me, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 1 } }),
    'POST /api/auth/activity': () => empty(204),
    'GET /api/auth/mfa/webauthn/passkeys': () => json(200, keys.slice(0, 1)),
    'DELETE /api/auth/mfa/webauthn/passkeys/k1': () =>
      json(409, {
        code: 'mfa_last_factor',
        message: "C'est votre dernier facteur : ajoutez-en un autre avant de le retirer.",
      }),
  })
  renderAt('/account')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Retirer Clé USB' }))
  expect((await screen.findByRole('alert')).textContent).toBe(
    "C'est votre dernier facteur : ajoutez-en un autre avant de le retirer.",
  )
  expect(screen.getByText('Clé USB')).toBeDefined()
})

test('an account guarded by a passkey adds an app without proof', async () => {
  let current = { ...me, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 1 } }
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, current),
    'POST /api/auth/activity': () => empty(204),
    'GET /api/auth/mfa/webauthn/passkeys': () => json(200, keys.slice(0, 1)),
    'POST /api/auth/mfa/totp/enroll': () => json(200, replacement),
    'POST /api/auth/mfa/totp/confirm': () => {
      current = { ...me, secondFactors: { totp: true, recoveryCodesRemaining: 10, passkeys: 1 } }
      return empty(204)
    },
  })
  renderAt('/account')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: "Ajouter l'application" }))
  await screen.findByRole('img', { name: "QR code de configuration de l'application" })
  expect(await calls.find((c) => c.url.endsWith('/totp/enroll'))?.json()).toEqual({})
  await user.type(screen.getByLabelText('Code à six chiffres'), '654321')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await user.click(await screen.findByRole('button', { name: "J'ai enregistré ces codes" }))
  expect(await screen.findByRole('button', { name: "Remplacer l'application" })).toBeDefined()
})

test('an account without passkey asks for none to be listed', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, me), 'POST /api/auth/activity': () => empty(204) })
  renderAt('/account')
  await screen.findByRole('heading', { name: "Clés d'accès" })
  expect(screen.getByText("Aucune clé d'accès sur ce compte.")).toBeDefined()
  expect(calls.some((c) => c.url.endsWith('/passkeys'))).toBe(false)
})

test('a list of passkeys that fails is told', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, keyed),
    'POST /api/auth/activity': () => empty(204),
    'GET /api/auth/mfa/webauthn/passkeys': () => new Response(null, { status: 500 }),
  })
  renderAt('/account')
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
})

test('the passkeys of the operator before are never shown to the next one', async () => {
  let current = keyed
  let signedIn = true
  let open: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    open = resolve
  })
  stubFetch({
    'GET /api/auth/me': () => (signedIn ? json(200, current) : noSession()),
    'POST /api/auth/activity': () => empty(204),
    'GET /api/auth/mfa/webauthn/passkeys': async () => {
      if (current !== keyed) {
        await gate
        return json(200, [{ id: 'k9', name: 'Clé de Autre', createdAt: '2026-10-20T12:00:00Z' }])
      }
      return json(200, keys)
    },
    'POST /api/auth/logout': () => {
      signedIn = false
      return empty(204)
    },
    'POST /api/auth/login': () => {
      signedIn = true
      return json(200, challenge)
    },
    'POST /api/auth/mfa/verify': () => empty(204),
  })
  renderAt('/account')
  const user = userEvent.setup()
  await screen.findByText('Clé USB')
  await user.click(screen.getByRole('button', { name: 'Se déconnecter' }))
  current = { ...keyed, operator: { ...keyed.operator, email: 'autre@example.test', displayName: 'Autre' } }
  await user.type(await screen.findByLabelText('Adresse e-mail'), 'autre@example.test')
  await user.type(screen.getByLabelText('Mot de passe'), 'Correct-horse-9')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await user.click(await screen.findByRole('link', { name: 'Autre' }))
  await screen.findByText('Connecté en tant que Autre (autre@example.test)')
  expect(screen.queryByText('Clé USB')).toBeNull()
  open()
  expect(await screen.findByText('Clé de Autre')).toBeDefined()
})

test('a logout forgets what the session had read', async () => {
  let signedIn = true
  stubFetch({
    'GET /api/auth/me': () => (signedIn ? json(200, { ...me, permissions: ['operators:read'] }) : noSession()),
    'GET /api/operators': () =>
      json(200, [
        {
          id: 'o-ada',
          email: 'ada@example.test',
          displayName: 'Ada',
          status: 'active',
          roles: [],
          secondFactorEnrolled: false,
          accessLink: null,
        },
      ]),
    'POST /api/auth/activity': () => empty(204),
    'POST /api/auth/logout': () => {
      signedIn = false
      return empty(204)
    },
  })
  const { router } = renderAt('/operators')
  await screen.findByText('ada@example.test')
  await userEvent.setup().click(screen.getByRole('button', { name: 'Se déconnecter' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  await act(async () => {
    await router.navigate({ to: '/operators' })
  })
  expect(screen.queryByText('ada@example.test')).toBeNull()
})

test('a session closed by the server forgets what it had read', async () => {
  stubFetch({
    'GET /api/auth/me': once(() => json(200, { ...me, permissions: ['operators:read'] })),
    'GET /api/operators': () =>
      json(200, [
        {
          id: 'o-ada',
          email: 'ada@example.test',
          displayName: 'Ada',
          status: 'active',
          roles: [],
          secondFactorEnrolled: false,
          accessLink: null,
        },
      ]),
    'POST /api/auth/activity': () => noSession(),
  })
  const { router } = renderAt('/operators')
  await screen.findByText('ada@example.test')
  fireEvent.keyDown(document)
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  await act(async () => {
    await router.navigate({ to: '/operators' })
  })
  expect(screen.queryByText('ada@example.test')).toBeNull()
})

const token = {
  token: 'eyJ.jeton-de-preuve.sig',
  scopes: ['accounts:read', 'traffic:read'],
  expiresAt: '2026-10-21T13:00:00Z',
}

const issuer = {
  ...me,
  permissions: ['gateway:personal_token', 'traffic:read', 'accounts:read', 'content:read', 'roles:read'],
}

test('a personal token is shown once, copied, and gone once closed', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, issuer),
    'POST /api/gateway-token': () => json(200, token),
  })
  const { container } = renderAt('/account')
  const user = userEvent.setup()
  expect(await screen.findByRole('heading', { name: 'Jeton d’API personnel' })).toBeDefined()
  const issue = screen.getByRole('button', { name: 'Délivrer' })
  await user.click(issue)
  const dialog = await screen.findByRole('dialog', { name: 'Jeton d’API personnel' })
  expect(dialog.textContent).toContain('eyJ.jeton-de-preuve.sig')
  expect(dialog.textContent).toContain('Ce jeton ne sera plus jamais affiché.')
  expect(screen.getByText(/expire une heure après sa délivrance/)).toBeDefined()
  expect(dialog.textContent).toContain('accounts:read, traffic:read')
  expect(await violations(container)).toEqual([])
  expect(within(dialog).getByRole('button', { name: 'Copier le secret' }).closest('.ui-modal__foot')).not.toBeNull()
  await user.click(within(dialog).getByRole('button', { name: 'Copier le secret' }))
  expect(await navigator.clipboard.readText()).toBe('eyJ.jeton-de-preuve.sig')
  await user.click(within(dialog).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.body.textContent).not.toContain('eyJ.jeton-de-preuve.sig')
  expect(document.activeElement).toBe(issue)
  await user.click(issue)
  await screen.findByRole('dialog')
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.body.textContent).not.toContain('eyJ.jeton-de-preuve.sig')
  const posted = calls.find((c) => c.url.endsWith('/api/gateway-token'))
  expect(posted?.headers.get('Content-Type')).toBe('application/json')
  expect(await posted?.text()).toBe('{"scopes":["accounts:read","traffic:read"]}')
})

test('a personal token carries the scopes checked, among those it may carry', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, issuer),
    'POST /api/gateway-token': () => json(200, token),
  })
  renderAt('/account')
  const user = userEvent.setup()
  const picker = await screen.findByRole('group', { name: 'Scopes du jeton' })
  const boxes = within(picker).getAllByRole('checkbox')
  expect(boxes.map((b) => (b.closest('label') as HTMLElement).textContent)).toEqual([
    expect.stringContaining('accounts:read'),
    expect.stringContaining('traffic:read'),
  ])
  expect(boxes.every((b) => b.getAttribute('aria-checked') === 'true')).toBe(true)
  await user.click(within(picker).getByRole('checkbox', { name: /traffic:read/ }))
  await user.click(screen.getByRole('button', { name: 'Délivrer' }))
  await screen.findByRole('dialog')
  const posted = calls.find((c) => c.url.endsWith('/api/gateway-token'))
  expect(await posted?.text()).toBe('{"scopes":["accounts:read"]}')
})

test('the scopes of a personal token are sent sorted, whatever the order they were checked in', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, issuer),
    'POST /api/gateway-token': () => json(200, token),
  })
  renderAt('/account')
  const user = userEvent.setup()
  const picker = await screen.findByRole('group', { name: 'Scopes du jeton' })
  await user.click(within(picker).getByRole('checkbox', { name: /accounts:read/ }))
  await user.click(within(picker).getByRole('checkbox', { name: /accounts:read/ }))
  await user.click(screen.getByRole('button', { name: 'Délivrer' }))
  await screen.findByRole('dialog')
  const posted = calls.find((c) => c.url.endsWith('/api/gateway-token'))
  expect(await posted?.text()).toBe('{"scopes":["accounts:read","traffic:read"]}')
})

test('a personal token with no scope checked is refused before it is asked', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, issuer) })
  renderAt('/account')
  const user = userEvent.setup()
  const picker = await screen.findByRole('group', { name: 'Scopes du jeton' })
  for (const box of within(picker).getAllByRole('checkbox')) {
    await user.click(box)
  }
  const issue = screen.getByRole('button', { name: 'Délivrer' })
  expect(issue.getAttribute('aria-disabled')).toBe('true')
  expect(document.getElementById(issue.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Cochez au moins un scope.',
  )
  await user.click(issue)
  expect(calls.some((c) => c.url.endsWith('/api/gateway-token'))).toBe(false)
})

test('a personal token refused or unavailable is told, without retry', async () => {
  let sent = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, issuer),
    'POST /api/gateway-token': () => {
      sent++
      return json(503, { code: 'service_unavailable', message: 'Le signataire des jetons est indisponible (serveur).' })
    },
  })
  renderAt('/account')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Délivrer' }))
  expect((await screen.findByRole('alert')).textContent).toBe('Le signataire des jetons est indisponible (serveur).')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(sent).toBe(1)
})

test('without gateway:personal_token, the delivery is refused and explained', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, me) })
  renderAt('/account')
  const issue = await screen.findByRole('button', { name: 'Délivrer' })
  expect(issue.getAttribute('aria-disabled')).toBe('true')
  expect(document.getElementById(issue.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Cette action demande Jeton personnel : se délivrer gateway:personal_token. Seul le rôle Propriétaire la détient par défaut ; un administrateur peut l’accorder.',
  )
  await userEvent.setup().click(issue)
  expect(calls.some((c) => c.url.endsWith('/api/gateway-token'))).toBe(false)
})
