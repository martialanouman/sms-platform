import { createFileRoute, redirect } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api } from '../api'
import type { components } from '../api.gen'
import { TotpEnrollment } from '../factor'
import { pendingChallenge, rememberChallenge, useChallengeExpiry, useElevated } from '../session'
import { AuthLayout, useSubmission } from '../ui'

export const Route = createFileRoute('/enroll')({
  beforeLoad: () => {
    if (!pendingChallenge()) {
      throw redirect({ to: '/login', search: { avis: 'verification-interrompue' } })
    }
  },
  component: Enrolment,
})

function Enrolment() {
  const elevated = useElevated()
  useChallengeExpiry()
  const { submit, waiting, refusal } = useSubmission()
  const [enrollment, setEnrollment] = useState<components['schemas']['TotpEnrollment']>()

  useEffect(() => {
    document.title = 'Second facteur — Tableau de bord'
  }, [])

  async function configure() {
    const outcome = await submit(() => api.POST('/auth/mfa/totp/enroll', { body: {} }))
    if (outcome.failure === undefined) {
      setEnrollment(outcome.data)
    }
  }

  return (
    <AuthLayout title="Second facteur">
      {enrollment ? (
        <TotpEnrollment
          enrollment={enrollment}
          confirm={async (code) => {
            const result = await api.POST('/auth/mfa/verify', {
              body: { challenge: pendingChallenge() ?? '', method: 'totp', code },
            })
            if (result.response.ok) {
              rememberChallenge(undefined)
            }
            return result
          }}
          onDone={elevated}
        />
      ) : (
        <>
          <p className="auth__intro">
            Votre compte demande un second facteur. Installez une application d'authentification sur votre téléphone,
            puis configurez-la.
          </p>
          {refusal}
          <button className="ui-button ui-button--primary" type="button" disabled={waiting} onClick={configure}>
            Configurer l'application
          </button>
        </>
      )}
    </AuthLayout>
  )
}
