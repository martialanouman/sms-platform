import { focusManager } from '@tanstack/react-query'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { rememberChallenge } from '../session'
import { challenge, empty, explained, json, me, once, renderAt, stubFetch, violations } from '../test'

const owner = { ...me, permissions: ['operators:read', 'operators:write', 'roles:read'] }
const support = { id: 'r-support', name: 'Support' }
const audit = { id: 'r-audit', name: 'Audit' }
const roles = [
  { ...audit, description: "Lit le journal d'audit, seul.", isDefault: true, permissions: ['audit:read'], holders: [] },
  { ...support, description: 'Lit les comptes.', isDefault: true, permissions: ['accounts:read'], holders: [] },
]
const self = {
  id: me.operator.id,
  email: me.operator.email,
  displayName: me.operator.displayName,
  status: 'active',
  roles: [{ id: 'r-owner', name: 'Propriétaire' }],
  secondFactorEnrolled: true,
  accessLink: null,
}
const ada = {
  id: 'o-ada',
  email: 'ada@example.test',
  displayName: 'Ada',
  status: 'active',
  roles: [support],
  secondFactorEnrolled: false,
  accessLink: { kind: 'activation', state: 'queued' },
}
const zed = {
  ...ada,
  id: 'o-zed',
  email: 'zed@example.test',
  displayName: 'Zed',
  status: 'disabled',
  roles: [],
  accessLink: null,
}
const links = [
  { link: { kind: 'activation', state: 'queued' }, text: 'Activation en attente' },
  { link: { kind: 'reset', state: 'queued' }, text: 'Envoi en attente' },
  { link: { kind: 'reset', state: 'sent' }, text: 'Lien envoyé' },
  { link: { kind: 'activation', state: 'failed' }, text: 'Envoi en échec' },
  { link: null, text: 'Actif' },
]

function row(email: string) {
  return screen.getByText(email).closest('tr') as HTMLTableRowElement
}

const cells = (email: string) =>
  within(row(email))
    .getAllByRole('cell')
    .map((c) => c.textContent)

const refocus = () =>
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })

test('the operators are listed with their status, roles and factor', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, owner), 'GET /api/operators': () => json(200, [self, ada, zed]) })
  const { container } = renderAt('/operators')
  expect(
    await within(await screen.findByRole('banner')).findByRole('heading', { name: 'Opérateurs', level: 1 }),
  ).toBeDefined()
  expect(await screen.findByRole('table', { name: 'Opérateurs du tableau de bord' })).toBeDefined()
  expect(document.title).toBe('Opérateurs — Tableau de bord')
  expect(within(row('ada@example.test')).getByRole('rowheader').textContent).toBe('Adaada@example.test')
  expect(cells('ada@example.test').slice(0, 3)).toEqual(['Activation en attente', 'Support', 'Aucun'])
  expect(cells('zed@example.test').slice(0, 3)).toEqual(['Désactivé', 'Aucun rôle', 'Aucun'])
  expect(cells('operateur@example.test').slice(0, 3)).toEqual(['Actif', 'Propriétaire', 'Configuré'])
  expect(await violations(container)).toEqual([])
})

test('each state of the access link is told', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () =>
      json(
        200,
        links.map((l, i) => ({ ...ada, id: `o-${i}`, email: `l${i}@example.test`, accessLink: l.link })),
      ),
  })
  renderAt('/operators')
  await screen.findByText('l0@example.test')
  expect(links.map((_, i) => cells(`l${i}@example.test`)[0])).toEqual(links.map((l) => l.text))
})

test('a list that cannot be read is told, with a retry', async () => {
  let failing = true
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => (failing ? new Response(null, { status: 500 }) : json(200, [self])),
  })
  renderAt('/operators')
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toContain('Les opérateurs n’ont pas pu être chargés')
  expect(alert.textContent).toContain('Le service ne répond pas ; réessayez.')
  failing = false
  await userEvent.setup().click(within(alert).getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByText('operateur@example.test')).toBeDefined()
})

test('an operator is created in a dialog, and a refusal is told there', async () => {
  let list: unknown[] = [self]
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, list),
    'POST /api/operators': async (r) => {
      const body = await r.clone().json()
      if (body.email === 'taken@example.test') {
        return json(409, { code: 'email_taken', message: 'Un opérateur a déjà cette adresse.' })
      }
      list = [self, ada]
      return json(201, ada)
    },
  })
  const { container } = renderAt('/operators')
  const user = userEvent.setup()
  const status = await screen.findByRole('status')
  expect(status.textContent).toBe('')
  const opener = await screen.findByRole('button', { name: 'Nouvel opérateur' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel opérateur' })
  expect(dialog.textContent).toContain('Le compte naît actif, sans rôle ni mot de passe')
  expect(await violations(container)).toEqual([])
  await user.type(within(dialog).getByLabelText('Adresse e-mail'), 'taken@example.test')
  await user.type(within(dialog).getByLabelText('Nom affiché'), 'Ada')
  await user.click(within(dialog).getByRole('button', { name: 'Créer l’opérateur' }))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Un opérateur a déjà cette adresse.')
  await user.clear(within(dialog).getByLabelText('Adresse e-mail'))
  await user.type(within(dialog).getByLabelText('Adresse e-mail'), 'Ada@Example.test')
  await user.click(within(dialog).getByRole('button', { name: 'Créer l’opérateur' }))
  expect(await screen.findByText('ada@example.test')).toBeDefined()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  expect(status.textContent).toBe('Compte créé. Le lien d’activation part à ada@example.test ; il vaut 72 heures.')
  const created = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/api/operators'))
  expect(await created[1]?.json()).toEqual({ email: 'Ada@Example.test', displayName: 'Ada' })
  expect(created[1]?.headers.get('Content-Type')).toBe('application/json')
})

test('a dialog closes by its close button, its cancel and its backdrop, and nothing is sent', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, owner), 'GET /api/operators': () => json(200, [self]) })
  renderAt('/operators')
  const user = userEvent.setup()
  const opener = await screen.findByRole('button', { name: 'Nouvel opérateur' })
  await user.click(opener)
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Fermer' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  await user.click(opener)
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Annuler' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  await user.click(opener)
  const dialog = await screen.findByRole('dialog')
  fireEvent.mouseDown(within(dialog).getByLabelText('Adresse e-mail'))
  fireEvent.mouseUp(dialog)
  fireEvent.click(dialog)
  expect(screen.getByRole('dialog')).toBe(dialog)
  await user.click(dialog)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  expect(calls.some((c) => c.method === 'POST' && c.url.includes('/api/operators'))).toBe(false)
})

test('the roles are replaced in a dialog that gives the focus back', async () => {
  let list: unknown[] = [self, ada]
  let reads = 0
  const calls = stubFetch({
    'GET /api/auth/me': () => {
      reads++
      return json(200, owner)
    },
    'GET /api/operators': () => json(200, list),
    'GET /api/roles': () => json(200, roles),
    'PUT /api/operators/o-ada/roles': () => {
      list = [self, { ...ada, roles: [audit, support] }]
      return json(200, list[1])
    },
    'PUT /api/operators/01a159b0-0000-7000-8000-000000000001/roles': () =>
      json(409, {
        code: 'self_lockout',
        message: 'Ce changement vous retirerait la gestion des opérateurs ou des rôles.',
      }),
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  const opener = within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Rôles de Ada' })
  expect(((await within(dialog).findByRole('checkbox', { name: /Support/ })) as HTMLInputElement).checked).toBe(true)
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
  await user.click(within(dialog).getByRole('checkbox', { name: /Audit/ }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer les rôles' }))
  await waitFor(() => expect(cells('ada@example.test')[1]).toBe('Audit, Support'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
  const put = calls.find((c) => c.method === 'PUT')
  expect(await put?.json()).toEqual({ roleIds: ['r-support', 'r-audit'] })

  const mine = within(row('operateur@example.test')).getByRole('button', { name: 'Modifier les rôles' })
  await user.click(mine)
  const own = await screen.findByRole('dialog', { name: 'Rôles de Opérateur' })
  await within(own).findByRole('checkbox', { name: /Support/ })
  await user.click(within(own).getByRole('button', { name: 'Enregistrer les rôles' }))
  expect((await within(own).findByRole('alert')).textContent).toBe(
    'Ce changement vous retirerait la gestion des opérateurs ou des rôles.',
  )
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(mine)
  expect(reads).toBe(1)
})

test('an account is disabled after a confirmation, then enabled directly', async () => {
  let list: unknown[] = [self, ada]
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, list),
    'PATCH /api/operators/o-ada': async (r) => {
      const { status } = await r.clone().json()
      list = [self, { ...ada, status }]
      return json(200, list[1])
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Désactiver' }))
  let dialog = await screen.findByRole('dialog', { name: 'Désactiver Ada ?' })
  expect(dialog.textContent).toContain('Ses sessions seront fermées immédiatement')
  expect(dialog.textContent).toContain('Ses rôles, son second facteur et un lien d’accès en cours resteront en place')
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
  await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
  expect(calls.some((c) => c.method === 'PATCH')).toBe(false)
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Désactiver' }))
  dialog = await screen.findByRole('dialog', { name: 'Désactiver Ada ?' })
  await user.click(within(dialog).getByRole('button', { name: 'Désactiver' }))
  await waitFor(() => expect(cells('ada@example.test')[0]).toBe('Désactivé'))
  expect(screen.queryByRole('dialog')).toBeNull()
  const enable = within(row('ada@example.test')).getByRole('button', { name: 'Réactiver' })
  expect(document.activeElement).toBe(enable)
  expect(enable.classList.contains('ui-button--danger')).toBe(false)
  const link = within(row('ada@example.test')).getByRole('button', { name: 'Envoyer un lien' })
  expect(link.getAttribute('aria-disabled')).toBe('true')
  expect(explained(link)).toBe('Le compte est désactivé : réactivez-le avant d’envoyer un lien.')
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Réactiver' }))
  await waitFor(() => expect(cells('ada@example.test')[0]).toBe('Activation en attente'))
  const patches = calls.filter((c) => c.method === 'PATCH')
  expect(await patches[0]?.json()).toEqual({ status: 'disabled' })
  expect(await patches[1]?.json()).toEqual({ status: 'active' })
})

test('a link is sent after a confirmation that tells its kind and length', async () => {
  const reset = {
    ...ada,
    id: 'o-bob',
    email: 'bob@example.test',
    displayName: 'Bob',
    accessLink: { kind: 'reset', state: 'sent' },
  }
  const bare = { ...ada, id: 'o-cy', email: 'cy@example.test', displayName: 'Cy', accessLink: null }
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [ada, reset, bare]),
    'POST /api/operators/o-ada/access-link': () => empty(202),
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  const open = async (email: string, name: string) => {
    await user.click(within(row(email)).getByRole('button', { name: 'Envoyer un lien' }))
    return screen.findByRole('dialog', { name })
  }
  let dialog = await open('bob@example.test', 'Envoyer un lien de réinitialisation à Bob ?')
  expect(dialog.textContent).toContain('il restera valable 1 heure')
  expect(dialog.textContent).toContain(
    'Son mot de passe, son second facteur et ses sessions resteront valables jusqu’à son usage.',
  )
  expect(dialog.textContent).not.toContain('Rien ne changera')
  await user.keyboard('{Escape}')
  dialog = await open('cy@example.test', 'Envoyer un lien à Cy ?')
  expect(dialog.textContent).toContain(
    'un lien à usage unique : activation (72 h) si le compte n’a pas encore de mot de passe, réinitialisation (1 h) sinon',
  )
  await user.keyboard('{Escape}')
  dialog = await open('ada@example.test', 'Envoyer un lien d’activation à Ada ?')
  expect(dialog.textContent).toContain('il restera valable 72 heures')
  expect(dialog.textContent).toContain('Le lien envoyé plus tôt cesse de valoir dès cette demande.')
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
})

test('a link is sent once at a time, and a refusal is told in its dialog', async () => {
  let release: () => void = () => {}
  let sent = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'POST /api/operators/o-ada/access-link': async () => {
      sent++
      await new Promise<void>((resolve) => (release = resolve))
      return json(409, { code: 'operator_disabled', message: 'Compte désactivé entre-temps (serveur).' })
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Envoyer un lien' }))
  const dialog = await screen.findByRole('dialog')
  const send = within(dialog).getByRole('button', { name: 'Envoyer' })
  await user.click(send)
  await user.click(send)
  expect(send.getAttribute('aria-busy')).toBe('true')
  expect(send.querySelector('.ui-button__spinner')).not.toBeNull()
  await act(async () => release())
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Compte désactivé entre-temps (serveur).')
  expect(sent).toBe(1)
})

test('what the operator may not do is disabled and explained', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, [self, ada, zed]),
  })
  const { container } = renderAt('/operators')
  await screen.findByText('ada@example.test')
  const user = userEvent.setup()
  const create = screen.getByRole('button', { name: 'Nouvel opérateur' })
  expect(create.getAttribute('aria-disabled')).toBe('true')
  expect(explained(create)).toBe('La création d’un opérateur demande operators:write.')
  create.focus()
  await user.keyboard('{Enter}')
  await user.click(create)
  expect(screen.queryByRole('dialog')).toBeNull()
  for (const name of ['Modifier les rôles', 'Envoyer un lien', 'Désactiver']) {
    const button = within(row('ada@example.test')).getByRole('button', { name })
    expect(button.getAttribute('aria-disabled')).toBe('true')
    expect(explained(button)).toBe('Cette action demande operators:write.')
  }
  const enable = within(row('zed@example.test')).getByRole('button', { name: 'Réactiver' })
  expect(explained(enable)).toBe('Cette action demande operators:write.')
  await user.click(enable)
  expect(calls.some((c) => c.method !== 'GET' && c.url.includes('/api/operators'))).toBe(false)
  expect(calls.some((c) => c.url.endsWith('/api/roles'))).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('roles cannot be set without reading them, and nobody disables himself', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read', 'operators:write'] }),
    'GET /api/operators': () => json(200, [self, ada]),
  })
  renderAt('/operators')
  await screen.findByText('ada@example.test')
  const roles = within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' })
  expect(roles.getAttribute('aria-disabled')).toBe('true')
  expect(explained(roles)).toBe('La liste des rôles à attribuer demande roles:read.')
  const disable = within(row('operateur@example.test')).getByRole('button', { name: 'Désactiver' })
  expect(disable.getAttribute('aria-disabled')).toBe('true')
  expect(explained(disable)).toBe(
    'Le compte de la session ne se désactive pas ici : un autre détenteur de operators:write peut le faire.',
  )
  expect(
    within(row('ada@example.test')).getByRole('button', { name: 'Désactiver' }).hasAttribute('aria-disabled'),
  ).toBe(false)
})

const mfaRequired = () => json(403, { code: 'mfa_required', message: "Vérifiez d'abord votre second facteur." })

test('an operation that requires the second factor leads to its verification', async () => {
  rememberChallenge(challenge)
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...owner, elevated: false }), 'GET /api/operators': mfaRequired })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/mfa'))
})

test('an operation that requires a second factor not enrolled leads to the enrolment', async () => {
  rememberChallenge(challenge)
  stubFetch({
    'GET /api/auth/me': () =>
      json(200, { ...owner, elevated: false, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 0 } }),
    'GET /api/operators': mfaRequired,
  })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/enroll'))
})

test('without a verification pending, a second factor required sends back to the login', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, { ...owner, elevated: false }), 'GET /api/operators': mfaRequired })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'verification-interrupted' })
})

test('an account is enabled once at a time, and a refusal is told on the screen', async () => {
  let release: () => void = () => {}
  let sent = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, zed]),
    'PATCH /api/operators/o-zed': async () => {
      sent++
      await new Promise<void>((resolve) => (release = resolve))
      return json(409, { code: 'conflict', message: 'Refusé (serveur).' })
    },
  })
  renderAt('/operators')
  await screen.findByText('zed@example.test')
  const user = userEvent.setup()
  const enable = within(row('zed@example.test')).getByRole('button', { name: 'Réactiver' })
  await user.click(enable)
  await user.click(enable)
  expect(enable.getAttribute('aria-busy')).toBe('true')
  await act(async () => release())
  expect((await within(row('zed@example.test')).findByRole('alert')).textContent).toBe('Refusé (serveur).')
  expect(sent).toBe(1)
  await user.click(enable)
  expect(screen.queryByRole('alert')).toBeNull()
  await act(async () => release())
  expect((await screen.findByRole('alert')).textContent).toBe('Refusé (serveur).')
})

test('the dialog survives the list read again while it is open', async () => {
  let list: unknown[] = [self, ada]
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, list),
    'GET /api/roles': () => json(200, roles),
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  const opener = within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Rôles de Ada' })
  list = [self, { ...ada, displayName: 'Ada L.' }]
  refocus()
  await screen.findByText('Ada L.')
  expect(dialog.hasAttribute('open')).toBe(true)
  await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(opener)
})

test('an operator who may not read the operators is told why, without a request', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['accounts:read'] }) })
  const { container } = renderAt('/operators')
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Nouvel opérateur' })).toBeNull()
  expect(calls.some((c) => c.url.endsWith('/api/operators'))).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('an ordinary refusal stays on the screen with its message', async () => {
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => {
      reads++
      return json(403, { code: 'forbidden_scope', message: 'Il faut la permission operators:read (serveur).' })
    },
  })
  const { router } = renderAt('/operators')
  expect((await screen.findByRole('alert')).textContent).toContain('Il faut la permission operators:read (serveur).')
  expect(router.state.location.pathname).toBe('/operators')
  expect(reads).toBe(1)
})

test('an account guarded by a passkey alone is sent to the verification', async () => {
  rememberChallenge(challenge)
  stubFetch({
    'GET /api/auth/me': () =>
      json(200, { ...owner, elevated: false, secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 1 } }),
    'GET /api/operators': mfaRequired,
  })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/mfa'))
})

test('taking operators:read away from oneself closes the dialog and tells why', async () => {
  let held = owner.permissions
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...owner, permissions: held }),
    'GET /api/operators': () => json(200, [self]),
    'GET /api/roles': () => json(200, roles),
    'PUT /api/operators/01a159b0-0000-7000-8000-000000000001/roles': () => {
      held = []
      return json(200, self)
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Modifier les rôles' }))
  const dialog = await screen.findByRole('dialog')
  await within(dialog).findByRole('checkbox', { name: /Support/ })
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer les rôles' }))
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Opérateurs', level: 1 }))
})

test('the list tells it is loading, then that it is empty', async () => {
  let answer: (r: Response) => void = () => {}
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => new Promise<Response>((resolve) => (answer = resolve)),
  })
  renderAt('/operators')
  expect(await screen.findByText('Chargement des opérateurs…')).toBeDefined()
  await act(async () => answer(json(200, [])))
  expect(await screen.findByText('Aucun opérateur')).toBeDefined()
  expect(screen.queryByText('Chargement des opérateurs…')).toBeNull()
})

test('the dialog tells the roles are loading, then that they cannot be read', async () => {
  let answer: (r: Response) => void = () => {}
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'GET /api/roles': () => new Promise<Response>((resolve) => (answer = resolve)),
  })
  renderAt('/operators')
  await screen.findByText('ada@example.test')
  await userEvent.setup().click(within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Chargement des rôles…')).toBeDefined()
  const save = within(dialog).getByRole('button', { name: 'Enregistrer les rôles' })
  expect(save.getAttribute('aria-disabled')).toBe('true')
  await act(async () => answer(new Response(null, { status: 500 })))
  const alert = await within(dialog).findByRole('alert')
  expect(alert.textContent).toContain('Les rôles n’ont pas pu être chargés')
  expect(alert.textContent).toContain('Le service ne répond pas ; réessayez.')
  expect(explained(within(dialog).getByRole('button', { name: 'Enregistrer les rôles' }))).toBe(
    'Les rôles n’ont pas pu être chargés : rien ne peut être enregistré.',
  )
  const user = userEvent.setup()
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer les rôles' }))
  await user.keyboard('{Escape}')
  expect(dialog.hasAttribute('open')).toBe(true)
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).toBeNull()
  let retried: (r: Response) => void = () => {}
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'GET /api/roles': () => {
      reads++
      return reads === 1 ? new Response(null, { status: 500 }) : new Promise<Response>((resolve) => (retried = resolve))
    },
  })
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' }))
  const again = await screen.findByRole('dialog')
  await within(again).findByRole('alert')
  await user.click(within(again).getByRole('button', { name: 'Réessayer' }))
  await act(async () => retried(json(200, roles)))
  expect(await within(again).findByRole('checkbox', { name: /Support/ })).toBeDefined()
})

test('a failure of the service with an error envelope is told as unreachable', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(503, { code: 'service_unavailable', message: 'Message interne (serveur).' }),
  })
  renderAt('/operators')
  expect((await screen.findByRole('alert')).textContent).toContain('Le service ne répond pas ; réessayez.')
})

test('a session gone while reading the list sends to the login', async () => {
  stubFetch({
    'GET /api/auth/me': once(() => json(200, owner)),
    'GET /api/operators': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }),
  })
  const { router } = renderAt('/operators')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ notice: 'session-closed' })
})

test('a dialog left open when the permission goes does not come back with it', async () => {
  let held = owner.permissions
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...owner, permissions: held }),
    'GET /api/operators': () => json(200, [self, ada]),
    'GET /api/roles': () => json(200, roles),
  })
  renderAt('/operators')
  await screen.findByText('ada@example.test')
  await userEvent.setup().click(within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' }))
  await screen.findByRole('dialog')
  held = []
  refocus()
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  held = owner.permissions
  refocus()
  await screen.findByText('ada@example.test')
  expect(document.querySelector('dialog')).toBeNull()
})

test('a success closes its own dialog at once, never one opened since', async () => {
  let release: () => void = () => {}
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'PATCH /api/operators/o-ada': async () => {
      await new Promise<void>((resolve) => (release = resolve))
      return json(200, { ...ada, status: 'disabled' })
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Désactiver' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Désactiver' }))
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Nouvel opérateur' }))
  await screen.findByRole('dialog', { name: 'Nouvel opérateur' })
  await act(async () => release())
  await waitFor(() => expect(calls.filter((c) => c.url.endsWith('/api/operators')).length).toBe(2))
  expect(screen.getByRole('dialog', { name: 'Nouvel opérateur' })).toBeDefined()
})

test('a link sent is not sent again while the list is read', async () => {
  let reads = 0
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => {
      reads++
      return reads === 1 ? json(200, [self, ada]) : new Promise<Response>(() => {})
    },
    'POST /api/operators/o-ada/access-link': () => empty(202),
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByText('ada@example.test')
  await user.click(within(row('ada@example.test')).getByRole('button', { name: 'Envoyer un lien' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Envoyer' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(calls.filter((c) => c.url.endsWith('/access-link')).length).toBe(1)
})

test('the reason of a blocked control is dismissed by Escape', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, [self]),
  })
  renderAt('/operators')
  const create = await screen.findByRole('button', { name: 'Nouvel opérateur' })
  const user = userEvent.setup()
  create.focus()
  const reason = create.parentElement as HTMLElement
  expect(reason.classList.contains('ui-blocked--dismissed')).toBe(false)
  await user.keyboard('{Escape}')
  expect(reason.classList.contains('ui-blocked--dismissed')).toBe(true)
  await user.tab()
  expect(reason.classList.contains('ui-blocked--dismissed')).toBe(false)
  await user.hover(create)
  await user.keyboard('{Escape}')
  expect(reason.classList.contains('ui-blocked--dismissed')).toBe(true)
  await user.unhover(create)
  expect(reason.classList.contains('ui-blocked--dismissed')).toBe(false)
})

test('a refusal to read the roles is told by its message', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'GET /api/roles': () =>
      json(403, { code: 'forbidden_scope', message: 'Il faut la permission roles:read (serveur).' }),
  })
  renderAt('/operators')
  await screen.findByText('ada@example.test')
  await userEvent.setup().click(within(row('ada@example.test')).getByRole('button', { name: 'Modifier les rôles' }))
  const dialog = await screen.findByRole('dialog')
  expect((await within(dialog).findByRole('alert')).textContent).toContain(
    'Il faut la permission roles:read (serveur).',
  )
})

test('an empty creation is refused field by field, in French and in colour, without a request', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, owner), 'GET /api/operators': () => json(200, [self]) })
  const { container } = renderAt('/operators')
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Nouvel opérateur' }))
  const dialog = await screen.findByRole('dialog', { name: 'Nouvel opérateur' })
  const email = within(dialog).getByLabelText('Adresse e-mail')
  const name = within(dialog).getByLabelText('Nom affiché')
  await user.type(email, 'a')
  expect(email.hasAttribute('aria-invalid')).toBe(false)
  await user.clear(email)
  await user.click(within(dialog).getByRole('button', { name: 'Créer l’opérateur' }))
  expect(document.activeElement).toBe(email)
  expect(email.getAttribute('aria-invalid')).toBe('true')
  expect(name.getAttribute('aria-invalid')).toBe('true')
  expect(explained(email)).toBe('Ce champ est obligatoire.')
  expect(explained(name)).toBe('Ce champ est obligatoire.')
  expect(
    within(dialog)
      .getAllByRole('alert')
      .map((a) => a.textContent),
  ).toEqual(['Ce champ est obligatoire.', 'Ce champ est obligatoire.'])
  expect(await violations(container)).toEqual([])
  await user.type(email, 'ada')
  expect(explained(email)).toBe('Adresse e-mail invalide.')
  await user.type(email, '@example.test')
  expect(email.hasAttribute('aria-invalid')).toBe(false)
  expect(explained(email)).toBeUndefined()
  expect(name.getAttribute('aria-invalid')).toBe('true')
  expect(calls.some((c) => c.method === 'POST' && c.url.includes('/api/operators'))).toBe(false)
})
