import createClient from 'openapi-fetch'
import type { components, paths } from './api.gen'

export type ApiError = components['schemas']['Error']

export const api = createClient<paths>({
  baseUrl: `${window.location.origin}/api`,
  fetch: (request) => globalThis.fetch(request),
})

export const json = { 'Content-Type': 'application/json' }
