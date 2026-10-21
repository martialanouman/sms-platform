import { createFileRoute, useNavigate, useRouterState } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { AuthLayout, Field, useSubmission } from '../ui'

export const Route = createFileRoute('/access')({ component: Access })

function Access() {
  const hash = useRouterState({ select: (state) => state.location.hash })
  const [token] = useState(hash)
  const navigate = useNavigate()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const password = useRef<HTMLInputElement>(null)

  useEffect(() => {
    document.title = 'Mot de passe — Tableau de bord'
    if (token) {
      void navigate({ to: '/access', hash: '', replace: true })
    }
    password.current?.focus()
  }, [token, navigate])

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const body = { token, password: String(new FormData(event.currentTarget).get('password') ?? '') }
    const { failure } = await submit(() => api.POST('/auth/access-link', { body }))
    if (failure === undefined) {
      await navigate({ to: '/login', search: { notice: 'password-set' } })
    }
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
      <form className="auth__form" noValidate onSubmit={save}>
        <Field
          label="Nouveau mot de passe"
          ref={password}
          name="password"
          type="password"
          autoComplete="new-password"
          required
          aria-describedby={describedBy}
        />
        {refusal}
        <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
          Enregistrer le mot de passe
        </button>
      </form>
    </AuthLayout>
  )
}
