import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, json, send, unreachable } from '../api'
import { TotpReplacement } from '../factor'
import { meQuery, Unauthenticated, useActivity, useMe, useSessionClosed } from '../session'
import { AuthLayout } from '../ui'

export const Route = createFileRoute('/')({ component: Home })

function Home() {
  const me = useMe()
  const closed = useSessionClosed()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [leaving, setLeaving] = useState(false)
  const [failure, setFailure] = useState('')
  useActivity(leaving ? undefined : me.data?.idleTimeoutSeconds)

  useEffect(() => {
    document.title = 'Tableau de bord'
  }, [])
  useEffect(() => {
    if (me.error instanceof Unauthenticated) {
      closed()
    }
  }, [me.error, closed])
  useEffect(() => {
    if (me.data && !me.data.elevated) {
      void navigate({ to: '/login', search: { avis: 'verification-interrompue' } })
    }
  }, [me.data, navigate])

  async function logout() {
    setFailure('')
    setLeaving(true)
    const { failure } = await send(() => api.POST('/auth/logout', { headers: json }))
    if (failure) {
      setLeaving(false)
      setFailure(failure)
      return
    }
    await navigate({ to: '/login' })
  }

  return (
    <AuthLayout title="Tableau de bord">
      {(failure || (me.error && !(me.error instanceof Unauthenticated))) && (
        <p className="auth__refusal" role="alert">
          {failure || unreachable}
        </p>
      )}
      {me.data?.elevated && (
        <>
          <p className="auth__intro">
            Connecté en tant que {me.data.operator.displayName} ({me.data.operator.email})
          </p>
          <button className="ui-button ui-button--secondary" type="button" onClick={logout}>
            Se déconnecter
          </button>
          {me.data.secondFactors.totp && (
            <TotpReplacement
              remaining={me.data.secondFactors.recoveryCodesRemaining}
              onReplaced={() => queryClient.invalidateQueries({ queryKey: meQuery.queryKey })}
            />
          )}
        </>
      )}
    </AuthLayout>
  )
}
