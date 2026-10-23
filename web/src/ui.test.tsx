import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef } from 'react'
import { expect, test, vi } from 'vitest'
import { explained } from './test'
import { Button, needs, Select, Toasts, useToast } from './ui'

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

test('a toast is a status, at most three stand, and each leaves after five seconds', async () => {
  vi.useFakeTimers()
  render(
    <Toasts>
      <Notifier />
    </Toasts>,
  )
  const region = screen.getByRole('region', { name: 'Notifications' })
  for (let i = 0; i < 4; i++) {
    act(() => fireEvent.click(screen.getByRole('button', { name: 'Notifier' })))
  }
  const standing = () => [...region.querySelectorAll('[role="status"]:not([data-limited])')].map((t) => t.textContent)
  expect(standing().sort()).toEqual(['Avis 2.', 'Avis 3.', 'Avis 4.'])
  act(() => vi.advanceTimersByTime(4900))
  expect(standing()).toHaveLength(3)
  await act(async () => vi.advanceTimersByTime(200))
  expect(region.querySelectorAll('[role="status"]')).toHaveLength(0)
})
