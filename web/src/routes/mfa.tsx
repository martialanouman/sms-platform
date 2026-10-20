import { createFileRoute, redirect } from '@tanstack/react-router'
import { useEffect } from 'react'
import { api } from '../api'
import { FactorProof } from '../factor'
import { pendingChallenge, useElevated } from '../session'
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

  useEffect(() => {
    document.title = 'Second facteur — Tableau de bord'
  }, [])

  return (
    <AuthLayout title="Second facteur">
      <p className="auth__intro">Saisissez le code de votre application d'authentification.</p>
      <FactorProof
        action="Vérifier"
        prove={(method, code) =>
          api.POST('/auth/mfa/verify', { body: { challenge: pendingChallenge() ?? '', method, code } })
        }
        onProved={elevated}
      />
    </AuthLayout>
  )
}
