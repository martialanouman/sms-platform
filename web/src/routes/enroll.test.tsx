import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QRCodeSVG } from 'qrcode.react'
import { act } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, test, vi } from 'vitest'
import { pendingChallenge, rememberChallenge } from '../session'
import { challenge, empty, json, me, renderAt, stubFetch, stubPasskeys, violations } from '../test'

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
  expect(router.state.location.search).toEqual({ notice: 'verification-interrupted' })
})

test('the app is configured, proved, and only then its codes are shown', async () => {
  rememberChallenge(challenge)
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
  expect(document.activeElement?.textContent).toMatch(/^Scannez ce QR code/)
  const drawn = new DOMParser().parseFromString(
    renderToStaticMarkup(<QRCodeSVG value={enrollment.otpauthUri} marginSize={4} />),
    'image/svg+xml',
  )
  expect(qr.querySelectorAll('path')[1]?.getAttribute('d')).toBe(drawn.querySelectorAll('path')[1]?.getAttribute('d'))
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
  rememberChallenge(challenge)
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
  rememberChallenge(challenge)
  stubFetch({ 'POST /api/auth/mfa/totp/enroll': () => new Response(null, { status: 502 }) })
  renderAt('/enroll')
  await configure()
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
  expect(screen.queryByRole('img')).toBeNull()
})

test('the key and the codes are copied, and a refused copy is told', async () => {
  rememberChallenge(challenge)
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

test('the buttons stay disabled while their request runs', async () => {
  let release: () => void = () => {}
  const held = () =>
    new Promise<void>((resolve) => {
      release = resolve
    })
  rememberChallenge(challenge)
  const calls = stubFetch({
    'POST /api/auth/mfa/totp/enroll': async () => {
      await held()
      return json(200, enrollment)
    },
    'POST /api/auth/mfa/verify': async () => {
      await held()
      return empty(204)
    },
  })
  renderAt('/enroll')
  const user = await configure()
  expect(screen.getByRole('button', { name: "Configurer l'application" })).toHaveProperty('disabled', true)
  await user.click(screen.getByRole('button', { name: "Configurer l'application" }))
  release()
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await waitFor(() => expect(calls.length).toBe(2))
  expect(screen.getByRole('button', { name: 'Vérifier' })).toHaveProperty('disabled', true)
  release()
  await screen.findByRole('heading', { name: 'Codes de récupération' })
})

test('the codes stay on screen once the challenge has expired', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  rememberChallenge({ challenge: challenge.challenge, expiresAt: new Date(Date.now() + 60_000).toISOString() })
  stubFetch({
    'POST /api/auth/mfa/totp/enroll': () => json(200, enrollment),
    'POST /api/auth/mfa/verify': () => empty(204),
  })
  const { router } = renderAt('/enroll')
  const user = await configure(userEvent.setup({ advanceTimers: vi.advanceTimersByTime }))
  await user.type(await screen.findByLabelText('Code à six chiffres'), '123456')
  await user.click(screen.getByRole('button', { name: 'Vérifier' }))
  await screen.findByRole('heading', { name: 'Codes de récupération' })
  await act(() => vi.advanceTimersByTimeAsync(120_000))
  expect(router.state.location.pathname).toBe('/enroll')
  expect(screen.getByRole('heading', { name: 'Codes de récupération' })).toBeDefined()
})

test('a second copy is announced again', async () => {
  rememberChallenge(challenge)
  stubFetch({ 'POST /api/auth/mfa/totp/enroll': () => json(200, enrollment) })
  renderAt('/enroll')
  const user = await configure()
  await user.click(await screen.findByRole('button', { name: 'Copier la clé' }))
  expect(screen.getByRole('status').textContent).toBe('Copié.')
  vi.spyOn(navigator.clipboard, 'writeText').mockReturnValue(new Promise(() => {}))
  await user.click(screen.getByRole('button', { name: 'Copier la clé' }))
  expect(screen.getByRole('status').textContent).toBe('')
})

const creation = { publicKey: { challenge: 'Y2hhbGxlbmdl', rp: { id: 'localhost', name: 'x' } } }

test('a passkey is registered under its name, then the verification follows', async () => {
  rememberChallenge(challenge)
  const device = stubPasskeys()
  const calls = stubFetch({
    'POST /api/auth/mfa/webauthn/register/begin': () => json(200, creation),
    'POST /api/auth/mfa/webauthn/register/finish': () => json(200, { id: 'k1' }),
  })
  const { router, container } = renderAt('/enroll')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Nom de la clé'), 'Clé USB')
  expect(await violations(container)).toEqual([])
  await user.click(screen.getByRole('button', { name: "Enregistrer une clé d'accès" }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/mfa'))
  expect(device.create).toHaveBeenCalledWith({ publicKey: { parsed: creation.publicKey } })
  const finish = calls.find((c) => c.url.endsWith('/register/finish'))
  expect(await finish?.json()).toEqual({ name: 'Clé USB', attestation: { id: 'attested' } })
  expect(pendingChallenge()).toBe(challenge.challenge)
})

test('a passkey abandoned or refused is told, and the app stays offered', async () => {
  rememberChallenge(challenge)
  const device = stubPasskeys()
  device.create.mockRejectedValueOnce(new DOMException('closed', 'NotAllowedError'))
  stubFetch({
    'POST /api/auth/mfa/webauthn/register/begin': () => json(200, creation),
    'POST /api/auth/mfa/webauthn/register/finish': () =>
      json(422, {
        code: 'webauthn_ceremony_refused',
        message: "La clé d'accès n'a pas été enregistrée ; recommencez.",
      }),
  })
  const { router } = renderAt('/enroll')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Nom de la clé'), 'Clé USB')
  await user.click(screen.getByRole('button', { name: "Enregistrer une clé d'accès" }))
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe(
    "La clé d'accès n'a pas répondu : la fenêtre s'est refermée, ou l'appareil a refusé. Recommencez.",
  )
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Nom de la clé').getAttribute('aria-describedby')).toBe(alert.id)
  await user.click(screen.getByRole('button', { name: "Enregistrer une clé d'accès" }))
  await waitFor(() =>
    expect(screen.getByRole('alert').textContent).toBe("La clé d'accès n'a pas été enregistrée ; recommencez."),
  )
  expect(router.state.location.pathname).toBe('/enroll')
  expect(screen.getByRole('button', { name: "Configurer l'application" })).toBeDefined()
})

test('a passkey without name is not attempted', async () => {
  rememberChallenge(challenge)
  stubPasskeys()
  const calls = stubFetch({})
  renderAt('/enroll')
  await userEvent.setup().click(await screen.findByRole('button', { name: "Enregistrer une clé d'accès" }))
  expect((await screen.findByRole('alert')).textContent).toBe('Nommez la clé : ce nom la distinguera des autres.')
  expect(calls).toEqual([])
})

test('a browser without the WebAuthn JSON methods is offered the app only', async () => {
  rememberChallenge(challenge)
  stubFetch({})
  renderAt('/enroll')
  expect(await screen.findByText("Ce navigateur ne sait pas créer de clé d'accès.")).toBeDefined()
  expect(screen.queryByLabelText('Nom de la clé')).toBeNull()
  expect(screen.getByRole('button', { name: "Configurer l'application" })).toBeDefined()
})

test('a name of spaces is not attempted', async () => {
  rememberChallenge(challenge)
  stubPasskeys()
  const calls = stubFetch({})
  renderAt('/enroll')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Nom de la clé'), '   ')
  await user.click(screen.getByRole('button', { name: "Enregistrer une clé d'accès" }))
  expect((await screen.findByRole('alert')).textContent).toBe('Nommez la clé : ce nom la distinguera des autres.')
  expect(calls).toEqual([])
})

test('after a login without factor, the registered passkey is offered at once', async () => {
  let passkeys = 0
  stubPasskeys()
  stubFetch({
    'POST /api/auth/login': () => json(200, challenge),
    'GET /api/auth/me': () =>
      json(200, { ...me, elevated: false, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys } }),
    'POST /api/auth/mfa/webauthn/register/begin': () => json(200, creation),
    'POST /api/auth/mfa/webauthn/register/finish': () => {
      passkeys = 1
      return json(200, { id: 'k1' })
    },
  })
  renderAt('/login')
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Adresse e-mail'), 'operateur@example.test')
  await user.type(screen.getByLabelText('Mot de passe'), 'Correct-horse-9')
  await user.click(screen.getByRole('button', { name: 'Se connecter' }))
  await user.type(await screen.findByLabelText('Nom de la clé'), 'Clé USB')
  await user.click(screen.getByRole('button', { name: "Enregistrer une clé d'accès" }))
  expect(await screen.findByRole('button', { name: "Utiliser une clé d'accès" })).toBeDefined()
})
