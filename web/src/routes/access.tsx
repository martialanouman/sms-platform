import { createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { api, send } from '../api'
import { AuthLayout, Field } from '../ui'

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
    const body = { token, password: String(new FormData(event.currentTarget).get('password') ?? '') }
    setError('')
    setWaiting(true)
    const { failure } = await send(() => api.POST('/auth/access-link', { body }))
    if (!failure) {
      await navigate({ to: '/login', search: { avis: 'mot-de-passe-enregistre' } })
      return
    }
    setError(failure)
    setWaiting(false)
  }

  if (!token) {
    return (
      <AuthLayout title="Définir un mot de passe">
        <p className="auth__refusal" role="alert">
          Ce lien est incomplet. Ouvrez celui du courriel reçu, en entier.
        </p>
      </AuthLayout>
    )
  }
  return (
    <AuthLayout title="Définir un mot de passe">
      <form className="auth__form" noValidate onSubmit={submit}>
        <Field
          label="Nouveau mot de passe"
          ref={password}
          name="password"
          type="password"
          autoComplete="new-password"
          required
          aria-describedby={error ? 'access-error' : undefined}
        />
        {error && (
          <p ref={alert} className="auth__refusal" id="access-error" role="alert" tabIndex={-1}>
            {error}
          </p>
        )}
        <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
          Enregistrer le mot de passe
        </button>
      </form>
    </AuthLayout>
  )
}
