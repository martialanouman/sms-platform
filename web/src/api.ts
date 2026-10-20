import createClient from 'openapi-fetch'
import type { paths } from './api.gen'

export const api = createClient<paths>({
  baseUrl: `${window.location.origin}/api`,
  fetch: (request) => globalThis.fetch(request),
})

export const json = { 'Content-Type': 'application/json' }

export const unreachable = 'Le service ne répond pas ; réessayez.'

type Outcome = { response: Response; failure?: undefined } | { response?: Response; failure: string }

export async function send(call: () => Promise<{ error?: unknown; response: Response }>): Promise<Outcome> {
  try {
    const { error, response } = await call()
    if (response.ok) {
      return { response }
    }
    const message = (error as { message?: unknown } | undefined)?.message
    return { response, failure: typeof message === 'string' ? message : unreachable }
  } catch {
    return { failure: unreachable }
  }
}
