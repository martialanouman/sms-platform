import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { PasskeySection, TotpReplacement } from '../factor'
import { meQuery, useShellMe } from '../session'

export const Route = createFileRoute('/_shell/')({ component: Account })

function Account() {
  const me = useShellMe()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const refreshMe = () => queryClient.invalidateQueries({ queryKey: meQuery.queryKey })

  useEffect(() => {
    document.title = 'Mon compte — Tableau de bord'
  }, [])
  useEffect(() => {
    if (me.data && !me.data.elevated) {
      void navigate({ to: '/login', search: { notice: 'verification-interrupted' } })
    }
  }, [me.data, navigate])

  return (
    <section className="page">
      <h1 className="page__title">Mon compte</h1>
      {me.data && (
        <>
          <p className="auth__intro">
            Connecté en tant que {me.data.operator.displayName} ({me.data.operator.email})
          </p>
          <TotpReplacement
            held={me.data.secondFactors.totp}
            remaining={me.data.secondFactors.recoveryCodesRemaining}
            onReplaced={refreshMe}
          />
          <PasskeySection count={me.data.secondFactors.passkeys} onChange={refreshMe} />
        </>
      )}
    </section>
  )
}
