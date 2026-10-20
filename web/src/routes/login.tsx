import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef } from 'react'
import { api, unreachable } from '../api'
import { meQuery, rememberChallenge } from '../session'
import { AuthLayout, Field, useSubmission } from '../ui'

const notices = {
  'session-fermee': 'Votre session est fermée. Reconnectez-vous.',
  'mot-de-passe-enregistre': 'Mot de passe enregistré. Vous pouvez vous connecter.',
  'verification-interrompue': 'La vérification a été interrompue. Reconnectez-vous.',
} as const

type Notice = keyof typeof notices

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { avis?: Notice } => ({
    avis: typeof search.avis === 'string' && Object.hasOwn(notices, search.avis) ? (search.avis as Notice) : undefined,
  }),
  component: Login,
})

function Login() {
  const { avis } = Route.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const email = useRef<HTMLInputElement>(null)

  useEffect(() => {
    document.title = 'Connexion — Tableau de bord'
    email.current?.focus()
  }, [])

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const body = { email: String(form.get('email') ?? ''), password: String(form.get('password') ?? '') }
    const outcome = await submit(() => api.POST('/auth/login', { body }))
    if (outcome.failure !== undefined) {
      return
    }
    rememberChallenge(outcome.data.challenge)
    queryClient.removeQueries({ queryKey: meQuery.queryKey })
    const current = await queryClient.fetchQuery(meQuery).catch(() => undefined)
    if (!current) {
      fail(unreachable)
      return
    }
    const { totp, passkeys } = current.secondFactors
    await navigate({ to: totp || passkeys > 0 ? '/mfa' : '/enroll' })
  }

  return (
    <AuthLayout title="Connexion">
      {avis && (
        <p className="auth__notice" role="status">
          {notices[avis]}
        </p>
      )}
      <form className="auth__form" noValidate onSubmit={login}>
        <Field
          label="Adresse e-mail"
          ref={email}
          name="email"
          type="email"
          autoComplete="username"
          required
          aria-describedby={describedBy}
        />
        <Field
          label="Mot de passe"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          aria-describedby={describedBy}
        />
        {refusal}
        <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
          Se connecter
        </button>
      </form>
    </AuthLayout>
  )
}
