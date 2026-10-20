import { screen } from '@testing-library/react'
import { act } from 'react'
import { expect, test } from 'vitest'
import { mount } from '../main'

test('the mounted SPA opens on the dashboard title', async () => {
  const root = document.createElement('div')
  document.body.append(root)
  await act(async () => mount(root))
  expect(await screen.findByRole('heading', { level: 1, name: 'Tableau de bord' })).toBeDefined()
})
