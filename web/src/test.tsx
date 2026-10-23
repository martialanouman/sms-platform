import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import axeCore from 'axe-core'
import { StrictMode } from 'react'
import { afterEach, expect, vi } from 'vitest'
import { routeTree } from './routeTree.gen'
import { rememberChallenge } from './session'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  Reflect.deleteProperty(navigator, 'credentials')
  vi.useRealTimers()
  rememberChallenge(undefined)
})

export function renderAt(path: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) })
  const view = render(
    <StrictMode>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>,
  )
  return { router, ...view }
}

type Handler = (request: Request) => Response | Promise<Response>

export function stubFetch(handlers: Record<string, Handler>) {
  const calls: Request[] = []
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init)
    const key = `${request.method} ${new URL(request.url).pathname}`
    calls.push(request)
    if (request.method !== 'GET' && request.headers.get('Content-Type')?.split(';')[0] !== 'application/json') {
      return json(415, { code: 'unsupported_media_type', message: 'La requête doit être en application/json.' })
    }
    const handler = handlers[key]
    if (!handler) {
      throw new Error(`unexpected request ${key}`)
    }
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
  return [...result.violations.map((v) => v.id), ...(container.querySelector('[title]') ? ['title'] : [])]
}

export const me = {
  operator: { id: '01a159b0-0000-7000-8000-000000000001', email: 'operateur@example.test', displayName: 'Opérateur' },
  permissions: [],
  elevated: true,
  secondFactors: { totp: true, recoveryCodesRemaining: 10, passkeys: 0 },
  idleTimeoutSeconds: 900,
  absoluteExpiresAt: '2026-10-21T02:00:00Z',
}

export const challenge = { challenge: 'c'.repeat(43), expiresAt: new Date(Date.now() + 300_000).toISOString() }

export function stubPasskeys() {
  vi.stubGlobal('PublicKeyCredential', {
    parseCreationOptionsFromJSON: (options: unknown) => ({ parsed: options }),
    parseRequestOptionsFromJSON: (options: unknown) => ({ parsed: options }),
  })
  const device = {
    create: vi.fn(async (_: unknown) => ({ toJSON: () => ({ id: 'attested' }) })),
    get: vi.fn(async (_: unknown) => ({ toJSON: () => ({ id: 'asserted' }) })),
  }
  Object.defineProperty(navigator, 'credentials', { configurable: true, value: device })
  return device
}

export const explained = (button: HTMLElement) =>
  document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent

export const noSession = () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' })

export function once(answer: () => Response) {
  let served = false
  return () => {
    if (served) {
      return noSession()
    }
    served = true
    return answer()
  }
}

export async function toasted() {
  const region = await screen.findByRole('region', { name: 'Notifications' })
  await waitFor(() => expect(within(region).queryAllByRole('status').length).toBeGreaterThan(0))
  return within(region)
    .getAllByRole('status')
    .map((t) => t.textContent)
}
