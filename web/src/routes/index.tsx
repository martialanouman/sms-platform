import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { api, json } from '../api'
import { Unauthenticated, useActivity, useMe, useSessionClosed } from '../session'

export const Route = createFileRoute('/')({ component: Home })

function Home() {
  const me = useMe()
  const closed = useSessionClosed()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  useActivity(me.data?.idleTimeoutSeconds)

  useEffect(() => {
    document.title = 'Tableau de bord'
  }, [])
  useEffect(() => {
    if (me.error instanceof Unauthenticated) {
      closed()
    }
  }, [me.error, closed])

  async function logout() {
    await api.POST('/auth/logout', { headers: json })
    queryClient.removeQueries({ queryKey: ['me'] })
    await navigate({ to: '/login' })
  }

  return (
    <main>
      <h1>Tableau de bord</h1>
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
