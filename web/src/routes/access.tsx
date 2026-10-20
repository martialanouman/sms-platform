import { createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { api } from '../api'

export const Route = createFileRoute('/access')({ component: Access })

function Access() {
  const hash = useRouterState({ select: (state) => state.location.hash })
  const [token] = useState(hash)
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [waiting, setWaiting] = useState(false)
  const password = useRef<HTMLInputElement>(null)
  const alert = useRef<HTMLParagraphElement>(null)

  useEffect(() => {
    document.title = 'Mot de passe — Tableau de bord'
    if (token) {
      void navigate({ to: '/access', hash: '', replace: true })
    }
    password.current?.focus()
  }, [token, navigate])
  useEffect(() => {
    if (error) {
      alert.current?.focus()
    }
  }, [error])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setWaiting(true)
    const { error: refused } = await api.POST('/auth/access-link', {
      body: { token, password: String(new FormData(event.currentTarget).get('password') ?? '') },
    })
    if (!refused) {
      await navigate({ to: '/login', search: { avis: 'mot-de-passe-enregistre' } })
      return
    }
    setError(refused.message)
    setWaiting(false)
  }

  if (!token) {
    return (
      <main>
        <h1>Définir un mot de passe</h1>
        <p role="alert">Ce lien est incomplet. Ouvrez celui du courriel reçu, en entier.</p>
      </main>
    )
  }
  return (
    <main>
      <h1>Définir un mot de passe</h1>
      <p id="password-rules">
        Douze caractères au moins, dont une majuscule, une minuscule, un chiffre et un caractère spécial.
      </p>
      <form noValidate onSubmit={submit}>
        <label htmlFor="password">Nouveau mot de passe</label>
        <input
          ref={password}
          id="password"
          name="password"
          type="password"
          autoComplete="new-password"
          required
          aria-describedby={error ? 'password-rules access-error' : 'password-rules'}
        />
        {error && (
          <p ref={alert} id="access-error" role="alert" tabIndex={-1}>
            {error}
          </p>
        )}
        <button type="submit" disabled={waiting}>
          Enregistrer le mot de passe
        </button>
      </form>
    </main>
  )
}
