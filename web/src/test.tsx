import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { cleanup, render } from '@testing-library/react'
import axeCore from 'axe-core'
import { afterEach, vi } from 'vitest'
import { routeTree } from './routeTree.gen'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

export function renderAt(path: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) })
  const view = render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { router, ...view }
}

type Handler = (request: Request) => Response | Promise<Response>

export function stubFetch(handlers: Record<string, Handler>) {
  const calls: Request[] = []
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init)
    const key = `${request.method} ${new URL(request.url).pathname}`
    const handler = handlers[key]
    if (!handler) {
      throw new Error(`unexpected request ${key}`)
    }
    calls.push(request)
    return handler(request)
  })
  return calls
}

export function json(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
}

export function empty(status: number) {
  return new Response(null, { status })
}

export async function violations(container: Element) {
  const result = await axeCore.run(container, { rules: { 'color-contrast': { enabled: false } } })
  return result.violations.map((v) => v.id)
}

export const me = {
  operator: { id: '01a159b0-0000-7000-8000-000000000001', email: 'operateur@example.test', displayName: 'Opérateur' },
  permissions: [],
  elevated: false,
  secondFactors: { totp: false, recoveryCodesRemaining: 0, passkeys: 0 },
  idleTimeoutSeconds: 900,
  absoluteExpiresAt: '2026-10-21T02:00:00Z',
}
