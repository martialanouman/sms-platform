import { createFileRoute, redirect } from '@tanstack/react-router'
import { useEffect } from 'react'
import { api, unreachable } from '../api'
import { FactorProof, PasskeyAssertion } from '../factor'
import { pendingChallenge, useChallengeExpiry, useElevated, useMe } from '../session'
import { AuthLayout } from '../ui'

export const Route = createFileRoute('/mfa')({
  beforeLoad: () => {
    if (!pendingChallenge()) {
      throw redirect({ to: '/login', search: { avis: 'verification-interrompue' } })
    }
  },
  component: Verification,
})

function Verification() {
  const elevated = useElevated()
  const me = useMe()
  useChallengeExpiry()

  useEffect(() => {
    document.title = 'Second facteur — Tableau de bord'
  }, [])

  return (
    <AuthLayout title="Second facteur">
      {me.error && (
        <p className="auth__refusal" role="alert">
          {unreachable}
        </p>
      )}
      {me.data && (
        <>
          <p className="auth__intro">Prouvez votre identité avec votre second facteur.</p>
          {me.data.secondFactors.passkeys > 0 && (
            <PasskeyAssertion challenge={pendingChallenge() ?? ''} onProved={elevated} />
          )}
          {me.data.secondFactors.totp && (
            <FactorProof
              action="Vérifier"
              prove={(method, code) =>
                api.POST('/auth/mfa/verify', { body: { challenge: pendingChallenge() ?? '', method, code } })
              }
              onProved={elevated}
            />
          )}
        </>
      )}
    </AuthLayout>
  )
}
