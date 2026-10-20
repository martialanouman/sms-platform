import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'
import { empty, json, renderAt, stubFetch, violations } from '../test'

const token = 'GPGV2vHfkxQ8j1p9vKs2qGKEnyJSCsPlKNC0_PMojzg'

async function submit(password: string) {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('Nouveau mot de passe'), password)
  await user.click(screen.getByRole('button', { name: 'Enregistrer le mot de passe' }))
}

test('the token leaves the address, then sets the password and sends to the login', async () => {
  const calls = stubFetch({ 'POST /api/auth/access-link': () => empty(204) })
  const { router } = renderAt(`/access#${token}`)
  await waitFor(() => expect(router.state.location.hash).toBe(''))
  expect(router.history.length).toBe(1)
  expect(document.title).toBe('Mot de passe — Tableau de bord')
  expect(document.activeElement).toBe(screen.getByLabelText('Nouveau mot de passe'))
  await submit('Correct-horse-9')
  await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
  expect(router.state.location.search).toEqual({ avis: 'mot-de-passe-enregistre' })
  expect(await calls[0]?.json()).toEqual({ token, password: 'Correct-horse-9' })
})

test.each([
  [410, 'access_link_invalid', "Ce lien n'est plus valable ; demandez-en un nouveau."],
  [422, 'password_policy', 'Il manque au mot de passe : une majuscule.'],
])('a refusal %i shows the server message', async (status, code, message) => {
  stubFetch({ 'POST /api/auth/access-link': () => json(status, { code, message }) })
  renderAt(`/access#${token}`)
  await submit('weak-password')
  const alert = await screen.findByRole('alert')
  expect(alert.textContent).toBe(message)
  expect(document.activeElement).toBe(alert)
  expect(screen.getByLabelText('Nouveau mot de passe').getAttribute('aria-describedby')).toBe(alert.id)
  expect(screen.getByRole('button', { name: 'Enregistrer le mot de passe' })).toHaveProperty('disabled', false)
})

test('a second identical refusal focuses the alert again', async () => {
  stubFetch({
    'POST /api/auth/access-link': () =>
      json(422, { code: 'password_policy', message: 'Il manque au mot de passe : une majuscule.' }),
  })
  renderAt(`/access#${token}`)
  await submit('weak-password')
  await screen.findByRole('alert')
  await submit('again')
  await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('alert')))
})

test('a proxy error without a body is not taken for a success', async () => {
  stubFetch({ 'POST /api/auth/access-link': () => new Response(null, { status: 502 }) })
  const { router } = renderAt(`/access#${token}`)
  await submit('Correct-horse-9')
  expect((await screen.findByRole('alert')).textContent).toBe('Le service ne répond pas ; réessayez.')
  expect(router.state.location.pathname).toBe('/access')
})

test('a link without its token sends nothing', async () => {
  const calls = stubFetch({})
  const { container } = renderAt('/access')
  expect((await screen.findByRole('alert')).textContent).toBe(
    'Ce lien est incomplet. Ouvrez celui du courriel reçu, en entier.',
  )
  expect(screen.queryByRole('button', { name: 'Enregistrer le mot de passe' })).toBeNull()
  expect(calls).toHaveLength(0)
  expect(await violations(container)).toEqual([])
})

test('the access screen has no accessibility violation', async () => {
  stubFetch({
    'POST /api/auth/access-link': () =>
      json(422, { code: 'password_policy', message: 'Il manque au mot de passe : une majuscule.' }),
  })
  const { container } = renderAt(`/access#${token}`)
  await screen.findByLabelText('Nouveau mot de passe')
  expect(await violations(container)).toEqual([])
  await submit('weak-password')
  await screen.findByRole('alert')
  expect(await act(() => violations(container))).toEqual([])
})
