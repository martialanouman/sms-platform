import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { routeTree } from './routeTree.gen'

test('the root route shows the dashboard title', async () => {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: ['/'] }) })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByRole('heading', { level: 1, name: 'Tableau de bord' })).toBeDefined()
})
