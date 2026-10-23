import createClient from 'openapi-fetch'
import type { paths } from './admin.gen'
import { mfaInterceptor } from './api'

export const gw = createClient<paths>({
  baseUrl: `${window.location.origin}/api/gw`,
  fetch: (request) => globalThis.fetch(request),
})

gw.use(mfaInterceptor, {
  onRequest({ request }) {
    if (request.method !== 'GET') {
      request.headers.set('Content-Type', 'application/json')
    }
    return request
  },
})
