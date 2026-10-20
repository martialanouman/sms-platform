import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, json, send, unreachable } from '../api'
import { Unauthenticated, useActivity, useMe, useSessionClosed } from '../session'

export const Route = createFileRoute('/')({ component: Home })

function Home() {
  const me = useMe()
  const closed = useSessionClosed()
  const navigate = useNavigate()
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
    <main>
      <h1>Tableau de bord</h1>
      {(failure || (me.error && !(me.error instanceof Unauthenticated))) && (
        <p role="alert">{failure || unreachable}</p>
      )}
      {me.data && (
        <>
          <p>
            Connecté en tant que {me.data.operator.displayName} ({me.data.operator.email})
          </p>
          <button type="button" onClick={logout}>
            Se déconnecter
          </button>
        </>
      )}
    </main>
  )
}
