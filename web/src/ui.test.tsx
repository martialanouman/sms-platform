import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef } from 'react'
import { expect, test, vi } from 'vitest'
import { explained } from './test'
import { Button, needs, Select, Switch, Tabs, Toasts, useToast } from './ui'

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
  expect(onChange).toHaveBeenCalledWith(true)
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
