import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode, useRef, useState } from 'react'
import { expect, test, vi } from 'vitest'
import { explained } from './test'
import {
  Button,
  Check,
  Choice,
  Choices,
  Modal,
  needs,
  RotationModal,
  SecretOnce,
  Select,
  Switch,
  Tabs,
  Toasts,
  useToast,
} from './ui'

const bubble = () => document.querySelector('.ui-tooltip')?.textContent

test('a blocked button explains itself in a bubble opened by the keyboard', async () => {
  render(<Button blockedReason="Déjà actif.">Réactiver</Button>)
  const button = screen.getByRole('button', { name: 'Réactiver' })
  expect(button.hasAttribute('disabled')).toBe(false)
  expect(button.getAttribute('aria-disabled')).toBe('true')
  expect(button.hasAttribute('title')).toBe(false)
  await userEvent.setup().tab()
  expect(document.activeElement).toBe(button)
  await waitFor(() => expect(bubble()).toBe('Déjà actif.'))
  expect(explained(button)).toBe('Déjà actif.')
})

test('a blocked select explains itself in a bubble, never in a title', async () => {
  render(
    <Select label="Groupe" items={[{ value: '', label: 'Tous' }]} value="" onChange={() => {}} blockedReason="Non." />,
  )
  const select = screen.getByRole('combobox', { name: 'Groupe' })
  expect(select.hasAttribute('title')).toBe(false)
  expect(select.getAttribute('aria-disabled')).toBe('true')
  await userEvent.setup().tab()
  expect(document.activeElement).toBe(select)
  await waitFor(() => expect(bubble()).toBe('Non.'))
  expect(explained(select)).toBe('Non.')
})

test('a permission reason names the permission in French and its key in mono', () => {
  render(<Button blockedReason={needs('customers:write')}>Nouveau client</Button>)
  const button = screen.getByRole('button', { name: 'Nouveau client' })
  expect(explained(button)).toBe('Cette action demande Clients : modifier customers:write.')
  const key = document.getElementById(button.getAttribute('aria-describedby') ?? '')?.querySelector('.role-keys__key')
  expect(key?.textContent).toBe('customers:write')
})

test('a permission reason may name its own subject', () => {
  render(<Button blockedReason={needs('roles:read', 'La liste des rôles')}>Modifier</Button>)
  expect(explained(screen.getByRole('button', { name: 'Modifier' }))).toBe(
    'La liste des rôles demande Rôles : lire roles:read.',
  )
})

function Notifier() {
  const notify = useToast()
  const n = useRef(0)
  return (
    <button type="button" onClick={() => notify(`Avis ${++n.current}.`)}>
      Notifier
    </button>
  )
}

test('toasts speak through their polite region, at most three stand, and each leaves after five seconds', async () => {
  vi.useFakeTimers()
  render(
    <Toasts>
      <Notifier />
    </Toasts>,
  )
  const region = screen.getByRole('region', { name: 'Notifications' })
  expect(region.getAttribute('aria-live')).toBe('polite')
  for (let i = 0; i < 4; i++) {
    act(() => fireEvent.click(screen.getByRole('button', { name: 'Notifier' })))
  }
  const standing = () => [...region.querySelectorAll('.ui-toast:not([data-limited])')].map((t) => t.textContent)
  expect(standing().sort()).toEqual(['Avis 2.', 'Avis 3.', 'Avis 4.'])
  expect(region.querySelectorAll('[role="status"]')).toHaveLength(0)
  act(() => vi.advanceTimersByTime(4999))
  expect(standing()).toHaveLength(3)
  await act(async () => vi.advanceTimersByTime(1))
  expect(region.querySelectorAll('.ui-toast')).toHaveLength(0)
})

test('tabs are links marking the current one', async () => {
  const root = createRootRoute({
    component: () => (
      <>
        <Tabs
          label="Journaux"
          tabs={[
            { to: '/audit/control-plane', label: 'Administration' },
            { to: '/audit/access', label: 'Accès' },
          ]}
        />
        <Outlet />
      </>
    ),
  })
  const page = (path: string) => createRoute({ getParentRoute: () => root, path, component: () => null })
  const router = createRouter({
    routeTree: root.addChildren([page('audit/control-plane'), page('audit/access')]),
    history: createMemoryHistory({ initialEntries: ['/audit/control-plane'] }),
  })
  render(<RouterProvider router={router} />)
  const nav = await screen.findByRole('navigation', { name: 'Journaux' })
  const current = await screen.findByRole('link', { name: 'Administration' })
  await waitFor(() => expect(current.getAttribute('aria-current')).toBe('page'))
  expect(nav.className).toBe('ui-tabs')
  await userEvent.setup().click(screen.getByRole('link', { name: 'Accès' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/audit/access'))
})

test('a free button carries no reason and opens no bubble, even focused', async () => {
  render(<Button>Suspendre</Button>)
  const button = screen.getByRole('button', { name: 'Suspendre' })
  await userEvent.setup().tab()
  expect(document.activeElement).toBe(button)
  expect(button.hasAttribute('aria-disabled')).toBe(false)
  expect(button.hasAttribute('aria-describedby')).toBe(false)
  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(document.querySelector('.ui-tooltip')).toBeNull()
})

test('a loading button stays inert', () => {
  render(<Button loading>Enregistrer</Button>)
  expect(screen.getByRole('button', { name: 'Enregistrer' }).getAttribute('aria-disabled')).toBe('true')
})

test('a free switch is named by its label, described by its explanation, and turns', async () => {
  const onChange = vi.fn()
  render(
    <Switch
      label="Canal REST"
      about="La clé d’API n’est acceptée que si ce canal est activé."
      checked={false}
      onChange={onChange}
    />,
  )
  const toggle = screen.getByRole('switch', { name: 'Canal REST' })
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect(explained(toggle)).toBe('La clé d’API n’est acceptée que si ce canal est activé.')
  expect(toggle.hasAttribute('aria-disabled')).toBe(false)
  await userEvent.setup().click(toggle)
  expect(onChange.mock.calls[0]?.[0]).toBe(true)
})

test('a blocked switch is described by its reason alone, opens its bubble, and does not turn', async () => {
  const onChange = vi.fn()
  render(
    <Switch
      label="Canal SMPP"
      about="Muet."
      checked
      onChange={onChange}
      blockedReason="Au moins un canal reste activé."
    />,
  )
  const toggle = screen.getByRole('switch', { name: 'Canal SMPP' })
  expect(toggle.getAttribute('aria-disabled')).toBe('true')
  expect(toggle.getAttribute('aria-describedby')?.split(' ')).toHaveLength(1)
  expect(explained(toggle)).toBe('Au moins un canal reste activé.')
  const user = userEvent.setup()
  await user.tab()
  expect(document.activeElement).toBe(toggle)
  await waitFor(() => expect(bubble()).toBe('Au moins un canal reste activé.'))
  await user.click(toggle)
  await user.keyboard(' ')
  expect(onChange).not.toHaveBeenCalled()
  expect(toggle.getAttribute('aria-checked')).toBe('true')
})

test('a busy switch is read-only and says so', async () => {
  const onChange = vi.fn()
  render(<Switch label="Canal REST" about="Muet." checked={false} onChange={onChange} busy />)
  const toggle = screen.getByRole('switch', { name: 'Canal REST' })
  expect(toggle.getAttribute('aria-busy')).toBe('true')
  expect(toggle.getAttribute('aria-readonly')).toBe('true')
  await userEvent.setup().click(toggle)
  expect(onChange).not.toHaveBeenCalled()
})

test('a blocked tab is a focusable button without href that explains itself', async () => {
  const root = createRootRoute({
    component: () => (
      <Tabs
        label="Sections"
        tabs={[
          { to: '/audit/access', label: 'A', blockedReason: 'Non.' },
          { to: '/audit/control-plane', label: 'B' },
        ]}
      />
    ),
  })
  const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ['/'] }) })
  render(<RouterProvider router={router} />)
  const blocked = await screen.findByRole('button', { name: 'A' })
  expect(blocked.className).toBe('ui-tabs__tab')
  expect(blocked.hasAttribute('href')).toBe(false)
  expect(screen.queryByRole('link', { name: 'A' })).toBeNull()
  expect(screen.getByRole('link', { name: 'B' })).toBeDefined()
  await userEvent.setup().tab()
  expect(document.activeElement).toBe(blocked)
  await waitFor(() => expect(bubble()).toBe('Non.'))
  expect(explained(blocked)).toBe('Non.')
})

test('a blocked tab is marked current when its route is the one shown', async () => {
  const root = createRootRoute({
    component: () => (
      <>
        <Tabs
          label="Sections"
          tabs={[
            { to: '/audit/access', label: 'A', blockedReason: 'Non.' },
            { to: '/audit/control-plane', label: 'B', blockedReason: 'Non.' },
          ]}
        />
        <Outlet />
      </>
    ),
  })
  const page = (path: string) => createRoute({ getParentRoute: () => root, path, component: () => null })
  const router = createRouter({
    routeTree: root.addChildren([page('audit/access'), page('audit/control-plane')]),
    history: createMemoryHistory({ initialEntries: ['/audit/access'] }),
  })
  render(<RouterProvider router={router} />)
  await waitFor(() => expect(screen.getByRole('button', { name: 'A' }).getAttribute('aria-current')).toBe('page'))
  expect(screen.getByRole('button', { name: 'B' }).hasAttribute('aria-current')).toBe(false)
})

function Opened({ chained }: { chained?: boolean }) {
  const [open, setOpen] = useState<'first' | 'second'>()
  const [gone, setGone] = useState(false)
  const origin = useRef<HTMLButtonElement>(null)
  return (
    <main id="contenu">
      <h1 tabIndex={-1}>Titre</h1>
      {!gone && (
        <button type="button" ref={origin} onClick={() => setOpen('first')}>
          Ouvrir
        </button>
      )}
      {open === 'first' && (
        <Modal
          title="Première"
          onClose={() => setOpen(undefined)}
          footer={
            <button
              type="button"
              onClick={() => {
                setGone(!chained)
                setOpen(chained ? 'second' : undefined)
              }}
            >
              Suite
            </button>
          }
        >
          Corps
        </Modal>
      )}
      {open === 'second' && (
        <SecretOnce title="Seconde" secret="s3cr3t" opener={origin.current} onClose={() => setOpen(undefined)}>
          Corps
        </SecretOnce>
      )}
    </main>
  )
}

test('a modal gives the focus back to its opener, or to the page title once the opener is gone', async () => {
  const user = userEvent.setup()
  render(<Opened />)
  await user.click(screen.getByRole('button', { name: 'Ouvrir' }))
  await screen.findByRole('dialog', { name: 'Première' })
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Ouvrir' }))
  await user.click(screen.getByRole('button', { name: 'Ouvrir' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Suite' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Titre' }))
})

test('a secret opened by another modal gives the focus back to the first opener, and says it will not be shown again', async () => {
  const user = userEvent.setup()
  render(
    <StrictMode>
      <Opened chained />
    </StrictMode>,
  )
  await user.click(screen.getByRole('button', { name: 'Ouvrir' }))
  await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Suite' }))
  const secret = await screen.findByRole('dialog', { name: 'Seconde' })
  const code = within(secret).getByText('s3cr3t')
  expect(code.nextElementSibling?.textContent).toBe(
    'Ce secret ne sera plus jamais affiché. Fermer cette fenêtre l’efface de l’écran.',
  )
  await user.click(within(secret).getByRole('button', { name: 'J’ai copié le secret' }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Ouvrir' }))
})

test('a check is named by its enclosing label and turns', async () => {
  const onChange = vi.fn()
  render(
    <label>
      <Check checked={false} onChange={onChange} />
      Je comprends.
    </label>,
  )
  const box = screen.getByRole('checkbox', { name: 'Je comprends.' })
  expect(box.className).toBe('ui-check')
  await userEvent.setup().click(box)
  expect(onChange.mock.calls[0]?.[0]).toBe(true)
})

function Rotating({ onSubmit }: { onSubmit: () => void }) {
  return (
    <RotationModal
      title={
        <>
          Rotation du bind SMPP <span className="ui-mono">acme01</span>
        </>
      }
      warning="Sans grâce, tout cesse."
      onClose={() => {}}
      onSubmit={onSubmit}
      waiting={false}
      refusal={<p role="alert">Refus.</p>}
    >
      <p>Champs</p>
    </RotationModal>
  )
}

test('a rotation warns, asks to understand first, and only then rotates', async () => {
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  render(<Rotating onSubmit={onSubmit} />)
  const dialog = await screen.findByRole('dialog', { name: 'Rotation du bind SMPP acme01' })
  expect(within(dialog).getByText('Sans grâce, tout cesse.').className).toBe('ui-banner--warn')
  expect(within(dialog).getByRole('alert').textContent).toBe('Refus.')
  expect(within(dialog).getByText('Champs')).toBeDefined()
  expect(dialog.textContent).toContain('L’action est enregistrée dans le journal d’audit.')
  const box = within(dialog).getByRole('checkbox', {
    name: 'Je comprends que le nouveau secret ne sera affiché qu’une seule fois.',
  })
  expect(box.getAttribute('aria-checked')).toBe('false')
  const rotate = within(dialog).getByRole('button', { name: 'Effectuer la rotation' })
  expect(rotate.className).toContain('ui-button--danger')
  expect(explained(rotate)).toBe('Cochez d’abord la case.')
  await user.click(rotate)
  expect(onSubmit).not.toHaveBeenCalled()
  await user.click(box)
  expect(rotate.hasAttribute('aria-disabled')).toBe(false)
  await user.click(rotate)
  expect(onSubmit).toHaveBeenCalledTimes(1)
})

test('a choice tells its hint in a bubble, on focus of its box and on hover of the option', async () => {
  render(
    <Choices value={[]} onChange={() => {}}>
      <Choice value="cdr:read" hint="cdr:read">
        Lire
      </Choice>
    </Choices>,
  )
  const box = screen.getByRole('checkbox', { name: 'Lire' })
  expect(document.querySelector('[title]')).toBeNull()
  await userEvent.setup().tab()
  expect(document.activeElement).toBe(box)
  await waitFor(() => expect(bubble()).toBe('cdr:read'))
  expect(explained(box)).toBe('cdr:read')
  fireEvent.keyDown(box, { key: 'Escape' })
  box.blur()
  await waitFor(() => expect(document.querySelector('.ui-tooltip')).toBeNull())
  await userEvent.setup().hover(screen.getByText('Lire'))
  await waitFor(() => expect(bubble()).toBe('cdr:read'))
})
