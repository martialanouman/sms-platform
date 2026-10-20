import { waitFor } from '@testing-library/react'
import { expect, test } from 'vitest'
import { json, stubFetch } from './test'

test('the mounted dashboard sends a closed session to the login at once', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }) })
  document.body.innerHTML = '<div id="root"></div>'
  await import('./main')
  await waitFor(() => expect(window.location.search).toBe('?avis=session-fermee'), { timeout: 500 })
  expect(window.location.pathname).toBe('/login')
  const styles = [...document.querySelectorAll('style')].map((s) => s.textContent).join('')
  expect(styles).toContain('.auth__card')
  expect(styles).toContain('--surface-page')
})
