import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { type FormEvent, useEffect, useRef } from 'react'
import { api } from '../api'
import { meQuery, rememberChallenge } from '../session'
import { AuthLayout, Field, useSubmission } from '../ui'

const notices = {
  'session-closed': 'Votre session est fermée. Reconnectez-vous.',
  'password-set': 'Mot de passe enregistré. Vous pouvez vous connecter.',
  'verification-interrupted': 'La vérification a été interrompue. Reconnectez-vous.',
} as const

type Notice = keyof typeof notices

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { notice?: Notice } => ({
    notice:
      typeof search.notice === 'string' && Object.hasOwn(notices, search.notice)
        ? (search.notice as Notice)
        : undefined,
  }),
  beforeLoad: async () => {
    const { data } = await api.GET('/auth/me').catch(() => ({ data: undefined }))
    if (data?.elevated) {
      throw redirect({ to: '/' })
    }
  },
  component: Login,
})

function Login() {
  const { notice } = Route.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const email = useRef<HTMLInputElement>(null)

  useEffect(() => {
    document.title = 'Connexion — Tableau de bord'
    email.current?.focus()
  }, [])

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const body = { email: String(form.get('email') ?? ''), password: String(form.get('password') ?? '') }
    const outcome = await submit(async () => {
      const result = await api.POST('/auth/login', { body })
      if (result.data) {
        rememberChallenge(result.data)
        return { ...result, data: await queryClient.fetchQuery(meQuery) }
      }
      return result
    })
    if (outcome.failure !== undefined) {
      return
    }
    const { totp, passkeys } = outcome.data.secondFactors
    await navigate({ to: totp || passkeys > 0 ? '/mfa' : '/enroll' })
  }

  return (
    <AuthLayout title="Connexion">
      {notice && (
        <p className="auth__notice" role="status">
          {notices[notice]}
        </p>
      )}
      <form className="auth__form" onSubmit={login}>
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
