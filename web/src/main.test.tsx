import { waitFor } from '@testing-library/react'
import { expect, test } from 'vitest'
import { Refused, retry, Unauthenticated } from './session'
import { json, stubFetch } from './test'

test('the mounted dashboard sends a closed session to the login at once', async () => {
  stubFetch({ 'GET /api/auth/me': () => json(401, { code: 'unauthenticated', message: 'Aucune session ouverte.' }) })
  document.body.innerHTML = '<div id="root"></div>'
  await import('./main')
  await waitFor(() => expect(window.location.search).toBe('?notice=session-closed'), { timeout: 500 })
  expect(window.location.pathname).toBe('/login')
  const styles = [...document.querySelectorAll('style')].map((s) => s.textContent).join('')
  expect(styles).toContain('.auth__card')
  expect(styles).toContain('--surface-page')
})

test('a refusal or a session gone is never retried, a failure up to three times', () => {
  expect(retry(0, new Refused('Il faut la permission.'))).toBe(false)
  expect(retry(0, new Unauthenticated())).toBe(false)
  expect(retry(2, new Error('down'))).toBe(true)
  expect(retry(3, new Error('down'))).toBe(false)
})
