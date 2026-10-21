import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { cleanup, render } from '@testing-library/react'
import axeCore from 'axe-core'
import { afterEach, vi } from 'vitest'
import { routeTree } from './routeTree.gen'
import { rememberChallenge } from './session'

const modals = new Set<HTMLDialogElement>()
const focus = HTMLElement.prototype.focus
HTMLElement.prototype.focus = function (this: HTMLElement, options?: FocusOptions) {
  const inert = [...modals].some((d) => d.isConnected && d.hasAttribute('open') && !d.contains(this))
  if (!inert) {
    focus.call(this, options)
  }
}

HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
  if (this.hasAttribute('open')) {
    throw new DOMException('The dialog is already open.', 'InvalidStateError')
  }
  modals.add(this)
  this.setAttribute('open', '')
  this.addEventListener('keydown', dismissOnEscape)
}

function dismissOnEscape(this: HTMLDialogElement, event: KeyboardEvent) {
  if (event.key === 'Escape' && this.dispatchEvent(new Event('cancel', { cancelable: true }))) {
    this.close()
  }
}
HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
  if (this.hasAttribute('open')) {
    this.removeAttribute('open')
    this.removeEventListener('keydown', dismissOnEscape)
    this.dispatchEvent(new Event('close'))
  }
}

afterEach(() => {
  cleanup()
  modals.clear()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  Reflect.deleteProperty(navigator, 'credentials')
  vi.useRealTimers()
  rememberChallenge(undefined)
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
    calls.push(request)
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
  return result.violations.map((v) => v.id)
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
