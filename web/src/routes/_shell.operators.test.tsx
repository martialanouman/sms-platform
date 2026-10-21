import { focusManager } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { rememberChallenge } from '../session'
import { challenge, empty, json, me, once, renderAt, stubFetch, violations } from '../test'

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
  { kind: 'reset', state: 'sent', text: 'Réinitialisation envoyée' },
  { kind: 'activation', state: 'failed', text: 'Activation en échec' },
]

function row(name: string) {
  const cell = screen.getByRole('rowheader', { name })
  return cell.closest('tr') as HTMLTableRowElement
}

test('the operators are listed with their roles, factor and link', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(200, owner), 'GET /api/operators': () => json(200, [self, ada, zed]) })
  const { container } = renderAt('/operators')
  expect(await screen.findByRole('heading', { name: 'Opérateurs', level: 1 })).toBeDefined()
  await screen.findByRole('rowheader', { name: 'Ada' })
  expect(document.title).toBe('Opérateurs — Tableau de bord')
  const cells = within(row('Ada'))
    .getAllByRole('cell')
    .map((c) => c.textContent)
  expect(cells).toEqual(['ada@example.test', 'Actif', 'Support', 'Non', 'Activation en file', expect.anything()])
  expect(
    within(row('Zed'))
      .getAllByRole('cell')
      .slice(1, 5)
      .map((c) => c.textContent),
  ).toEqual(['Désactivé', 'Aucun', 'Non', '—'])
  expect(within(row('Opérateur')).getAllByRole('cell')[3]?.textContent).toBe('Oui')
  expect(await violations(container)).toEqual([])
})

test('a list that cannot be read is told, with a retry', async () => {
  let failing = true
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => (failing ? new Response(null, { status: 500 }) : json(200, [self])),
  })
  renderAt('/operators')
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
  failing = false
  await userEvent.setup().click(screen.getByRole('button', { name: 'Réessayer' }))
  expect(await screen.findByRole('rowheader', { name: 'Opérateur' })).toBeDefined()
})

test('an operator is created, and a refusal is told by its message', async () => {
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
  renderAt('/operators')
  const user = userEvent.setup()
  const form = await screen.findByRole('form', { name: 'Nouvel opérateur' })
  await user.type(within(form).getByLabelText('Adresse e-mail'), 'taken@example.test')
  await user.type(within(form).getByLabelText('Nom'), 'Ada')
  await user.click(within(form).getByRole('button', { name: 'Créer' }))
  expect((await within(form).findByRole('alert')).textContent).toBe('Un opérateur a déjà cette adresse.')
  await user.clear(within(form).getByLabelText('Adresse e-mail'))
  await user.type(within(form).getByLabelText('Adresse e-mail'), 'ada@example.test')
  await user.click(within(form).getByRole('button', { name: 'Créer' }))
  expect(await screen.findByRole('rowheader', { name: 'Ada' })).toBeDefined()
  expect((within(form).getByLabelText('Adresse e-mail') as HTMLInputElement).value).toBe('')
  expect((within(form).getByLabelText('Nom') as HTMLInputElement).value).toBe('')
  const created = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/api/operators'))
  expect(await created[1]?.json()).toEqual({ email: 'ada@example.test', displayName: 'Ada' })
  expect(created[1]?.headers.get('Content-Type')).toBe('application/json')
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
  await screen.findByRole('rowheader', { name: 'Ada' })
  const opener = within(row('Ada')).getByRole('button', { name: 'Rôles' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Rôles de Ada' })
  expect((within(dialog).getByRole('checkbox', { name: /Support/ }) as HTMLInputElement).checked).toBe(true)
  await user.click(within(dialog).getByRole('checkbox', { name: /Audit/ }))
  await user.click(within(dialog).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(within(row('Ada')).getAllByRole('cell')[2]?.textContent).toBe('Audit, Support'))
  expect(dialog.hasAttribute('open')).toBe(false)
  expect(document.activeElement).toBe(opener)
  const put = calls.find((c) => c.method === 'PUT')
  expect(await put?.json()).toEqual({ roleIds: ['r-support', 'r-audit'] })

  const mine = within(row('Opérateur')).getByRole('button', { name: 'Rôles' })
  await user.click(mine)
  const own = await screen.findByRole('dialog', { name: 'Rôles de Opérateur' })
  await user.click(within(own).getByRole('button', { name: 'Enregistrer' }))
  expect((await within(own).findByRole('alert')).textContent).toBe(
    'Ce changement vous retirerait la gestion des opérateurs ou des rôles.',
  )
  await user.keyboard('{Escape}')
  expect(own.hasAttribute('open')).toBe(false)
  expect(document.activeElement).toBe(mine)
  expect(reads).toBe(1)
})

test('a change on oneself reads the session again', async () => {
  let reads = 0
  stubFetch({
    'GET /api/auth/me': () => {
      reads++
      return json(200, owner)
    },
    'GET /api/operators': () => json(200, [self]),
    'GET /api/roles': () => json(200, roles),
    'PUT /api/operators/01a159b0-0000-7000-8000-000000000001/roles': () => json(200, self),
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await user.click(within(await screen.findByRole('row', { name: /Opérateur/ })).getByRole('button', { name: 'Rôles' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Enregistrer' }))
  await waitFor(() => expect(reads).toBe(2))
})

test('a link is sent and an account is disabled then enabled', async () => {
  let list: unknown[] = [self, ada]
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, list),
    'POST /api/operators/o-ada/access-link': () => empty(202),
    'PATCH /api/operators/o-ada': async (r) => {
      const { status } = await r.clone().json()
      list = [self, { ...ada, status }]
      return json(200, list[1])
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Ada' })
  await user.click(within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' }))
  expect(await within(row('Ada')).findByText('Lien demandé.')).toBeDefined()
  await user.click(within(row('Ada')).getByRole('button', { name: 'Désactiver' }))
  await waitFor(() => expect(within(row('Ada')).getAllByRole('cell')[1]?.textContent).toBe('Désactivé'))
  const link = within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' })
  expect(link.hasAttribute('disabled')).toBe(true)
  expect(document.getElementById(link.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Aucun lien ne part vers un compte désactivé.',
  )
  await user.click(within(row('Ada')).getByRole('button', { name: 'Réactiver' }))
  await waitFor(() => expect(within(row('Ada')).getAllByRole('cell')[1]?.textContent).toBe('Actif'))
  const patches = calls.filter((c) => c.method === 'PATCH')
  expect(await patches[0]?.json()).toEqual({ status: 'disabled' })
  expect(await patches[1]?.json()).toEqual({ status: 'active' })
})

test('what the operator may not do is disabled and explained', async () => {
  const calls = stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read'] }),
    'GET /api/operators': () => json(200, [self, ada]),
  })
  const { container } = renderAt('/operators')
  await screen.findByRole('rowheader', { name: 'Ada' })
  const explained = (button: HTMLElement) =>
    document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent
  const create = within(screen.getByRole('form', { name: 'Nouvel opérateur' })).getByRole('button', { name: 'Créer' })
  expect(create.hasAttribute('disabled')).toBe(true)
  expect(explained(create)).toBe('Il faut la permission operators:write.')
  for (const name of ['Rôles', 'Envoyer un lien', 'Désactiver']) {
    const button = within(row('Ada')).getByRole('button', { name })
    expect(button.hasAttribute('disabled')).toBe(true)
    expect(explained(button)).toBe('Il faut la permission operators:write.')
  }
  expect(calls.some((c) => c.url.endsWith('/api/roles'))).toBe(false)
  expect(await violations(container)).toEqual([])
})

test('roles cannot be set without reading them, and nobody disables himself', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...me, permissions: ['operators:read', 'operators:write'] }),
    'GET /api/operators': () => json(200, [self, ada]),
  })
  renderAt('/operators')
  await screen.findByRole('rowheader', { name: 'Ada' })
  const explained = (button: HTMLElement) =>
    document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent
  const roles = within(row('Ada')).getByRole('button', { name: 'Rôles' })
  expect(roles.hasAttribute('disabled')).toBe(true)
  expect(explained(roles)).toBe('Il faut la permission roles:read.')
  const disable = within(row('Opérateur')).getByRole('button', { name: 'Désactiver' })
  expect(disable.hasAttribute('disabled')).toBe(true)
  expect(explained(disable)).toBe('Vous ne pouvez pas vous désactiver.')
  expect(within(row('Ada')).getByRole('button', { name: 'Désactiver' }).hasAttribute('disabled')).toBe(false)
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

test('a refusal of a row action is told in the row', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'POST /api/operators/o-ada/access-link': () =>
      json(409, { code: 'operator_disabled', message: 'Compte désactivé entre-temps (serveur).' }),
  })
  renderAt('/operators')
  await screen.findByRole('rowheader', { name: 'Ada' })
  await userEvent.setup().click(within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' }))
  expect((await within(row('Ada')).findByRole('alert')).textContent).toBe('Compte désactivé entre-temps (serveur).')
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
  await screen.findByRole('rowheader', { name: 'Ada' })
  const opener = within(row('Ada')).getByRole('button', { name: 'Rôles' })
  await user.click(opener)
  const dialog = await screen.findByRole('dialog', { name: 'Rôles de Ada' })
  list = [self, { ...ada, displayName: 'Ada L.' }]
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await screen.findByRole('rowheader', { name: 'Ada L.' })
  expect(dialog.hasAttribute('open')).toBe(true)
  await user.click(within(dialog).getByRole('button', { name: 'Annuler' }))
  expect(dialog.hasAttribute('open')).toBe(false)
  expect(document.activeElement).toBe(opener)
})

test('an operator who may not read the operators is told why, without a request', async () => {
  const calls = stubFetch({ 'GET /api/auth/me': () => json(200, { ...me, permissions: ['accounts:read'] }) })
  const { container } = renderAt('/operators')
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  expect(screen.queryByRole('form')).toBeNull()
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
  expect((await screen.findByRole('alert')).textContent).toBe('Il faut la permission operators:read (serveur).')
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
  await user.click(within(await screen.findByRole('row', { name: /Opérateur/ })).getByRole('button', { name: 'Rôles' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Enregistrer' }))
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Opérateurs', level: 1 }))
  held = owner.permissions
  act(() => {
    focusManager.setFocused(false)
    focusManager.setFocused(true)
  })
  await screen.findByRole('rowheader', { name: 'Opérateur' })
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('each kind and state of access link is told', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () =>
      json(
        200,
        links.map((l, i) => ({
          ...ada,
          id: `o-${i}`,
          displayName: `L${i}`,
          accessLink: { kind: l.kind, state: l.state },
        })),
      ),
  })
  renderAt('/operators')
  for (const [i, l] of links.entries()) {
    expect(
      within((await screen.findByRole('rowheader', { name: `L${i}` })).closest('tr') as HTMLElement).getAllByRole(
        'cell',
      )[4]?.textContent,
    ).toBe(l.text)
  }
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
  expect(await screen.findByText('Aucun opérateur.')).toBeDefined()
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
  await screen.findByRole('rowheader', { name: 'Ada' })
  await userEvent.setup().click(within(row('Ada')).getByRole('button', { name: 'Rôles' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Chargement des rôles…')).toBeDefined()
  expect(within(dialog).getByRole('button', { name: 'Enregistrer' }).hasAttribute('disabled')).toBe(true)
  await act(async () => answer(new Response(null, { status: 500 })))
  expect((await within(dialog).findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
  expect(within(dialog).getByRole('button', { name: 'Enregistrer' }).hasAttribute('disabled')).toBe(true)
})

test('a row action runs once at a time, and a new one forgets the last outcome', async () => {
  let release: () => void = () => {}
  let sent = 0
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'POST /api/operators/o-ada/access-link': async () => {
      sent++
      if (sent === 1) {
        await new Promise<void>((resolve) => (release = resolve))
        return empty(202)
      }
      return json(409, { code: 'operator_disabled', message: 'Refusé (serveur).' })
    },
  })
  renderAt('/operators')
  const user = userEvent.setup()
  await screen.findByRole('rowheader', { name: 'Ada' })
  const link = within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' })
  await user.click(link)
  await user.click(link)
  expect(link.getAttribute('aria-disabled')).toBe('true')
  expect(document.activeElement).toBe(link)
  await act(async () => release())
  expect(await within(row('Ada')).findByText('Lien demandé.')).toBeDefined()
  expect(sent).toBe(1)
  await user.click(within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' }))
  expect((await within(row('Ada')).findByRole('alert')).textContent).toBe('Refusé (serveur).')
  expect(within(row('Ada')).queryByText('Lien demandé.')).toBeNull()
})

test('a failure of the service with an error envelope is told as unreachable', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(503, { code: 'service_unavailable', message: 'Message interne (serveur).' }),
  })
  renderAt('/operators')
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
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

test('the outcome of a row action is told in a status region present before it', async () => {
  stubFetch({
    'GET /api/auth/me': () => json(200, owner),
    'GET /api/operators': () => json(200, [self, ada]),
    'POST /api/operators/o-ada/access-link': () => empty(202),
  })
  renderAt('/operators')
  await screen.findByRole('rowheader', { name: 'Ada' })
  const status = within(row('Ada')).getByRole('status')
  expect(status.textContent).toBe('')
  await userEvent.setup().click(within(row('Ada')).getByRole('button', { name: 'Envoyer un lien' }))
  await waitFor(() => expect(status.textContent).toBe('Lien demandé.'))
  expect(within(row('Ada')).getByRole('status')).toBe(status)
})

test('a dialog left open when the permission goes does not come back with it', async () => {
  let held = owner.permissions
  stubFetch({
    'GET /api/auth/me': () => json(200, { ...owner, permissions: held }),
    'GET /api/operators': () => json(200, [self, ada]),
    'GET /api/roles': () => json(200, roles),
  })
  renderAt('/operators')
  await screen.findByRole('rowheader', { name: 'Ada' })
  await userEvent.setup().click(within(row('Ada')).getByRole('button', { name: 'Rôles' }))
  await screen.findByRole('dialog')
  const refocus = () =>
    act(() => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })
  held = []
  refocus()
  expect(await screen.findByText('Il faut la permission operators:read pour lire les opérateurs.')).toBeDefined()
  held = owner.permissions
  refocus()
  await screen.findByRole('rowheader', { name: 'Ada' })
  expect(document.querySelector('dialog')).toBeNull()
})
