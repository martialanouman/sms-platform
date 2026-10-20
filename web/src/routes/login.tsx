import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { api } from '../api'

const notices = {
  'session-fermee': 'Votre session est fermée. Reconnectez-vous.',
  'mot-de-passe-enregistre': 'Mot de passe enregistré. Vous pouvez vous connecter.',
} as const

type Notice = keyof typeof notices

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { avis?: Notice } =>
    typeof search.avis === 'string' && search.avis in notices ? { avis: search.avis as Notice } : {},
  component: Login,
})

function Login() {
  const { avis } = Route.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [error, setError] = useState('')
  const [waiting, setWaiting] = useState(false)
  const email = useRef<HTMLInputElement>(null)
  const alert = useRef<HTMLParagraphElement>(null)

  useEffect(() => {
    document.title = 'Connexion — Tableau de bord'
    email.current?.focus()
  }, [])
  useEffect(() => {
    if (error) {
      alert.current?.focus()
    }
  }, [error])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const body = { email: String(form.get('email') ?? ''), password: String(form.get('password') ?? '') }
    setWaiting(true)
    const { error: refused, response } = await api.POST('/auth/login', { body })
    if (!refused) {
      queryClient.removeQueries({ queryKey: ['me'] })
      await navigate({ to: '/' })
      return
    }
    setError(refused.message)
    const retry = Number(response.headers.get('Retry-After'))
    if (response.status === 429 && retry > 0) {
      setTimeout(() => setWaiting(false), retry * 1000)
      return
    }
    setWaiting(false)
  }

  return (
    <main>
      <h1>Connexion</h1>
      {avis && <p role="status">{notices[avis]}</p>}
      <form noValidate onSubmit={submit}>
        <label htmlFor="email">Adresse e-mail</label>
        <input
          ref={email}
          id="email"
          name="email"
          type="email"
          autoComplete="username"
          required
          aria-describedby={error ? 'login-error' : undefined}
        />
        <label htmlFor="password">Mot de passe</label>
        <input
          id="password"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          aria-describedby={error ? 'login-error' : undefined}
        />
        {error && (
          <p ref={alert} id="login-error" role="alert" tabIndex={-1}>
            {error}
          </p>
        )}
        <button type="submit" disabled={waiting}>
          Se connecter
        </button>
      </form>
    </main>
  )
}
