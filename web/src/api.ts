import createClient from 'openapi-fetch'
import type { paths } from './api.gen'

export const api = createClient<paths>({
  baseUrl: `${window.location.origin}/api`,
  fetch: (request) => globalThis.fetch(request),
})

export const json = { 'Content-Type': 'application/json' }

export const unreachable = 'Le service ne répond pas ; réessayez.'

type Outcome<T> = { response: Response; data: T; failure?: undefined } | { response?: Response; failure: string }

export async function send<T>(
  call: () => Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<Outcome<T>> {
  try {
    const { data, error, response } = await call()
    if (response.ok) {
      return { response, data: data as T }
    }
    const message = (error as { message?: unknown } | undefined)?.message
    return { response, failure: typeof message === 'string' ? message : unreachable }
  } catch {
    return { failure: unreachable }
  }
}
