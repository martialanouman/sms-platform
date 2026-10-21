import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useShellMe } from '../session'
import { EmptyState } from '../ui'

export const Route = createFileRoute('/_shell/')({ component: Home })

function Home() {
  const me = useShellMe()
  const navigate = useNavigate()

  useEffect(() => {
    document.title = 'Accueil — Tableau de bord'
  }, [])
  useEffect(() => {
    // debt: #155
    if (me.data && !me.data.elevated) {
      void navigate({ to: '/login', search: { notice: 'verification-interrupted' } })
    }
  }, [me.data, navigate])

  return (
    <div className="page">
      <EmptyState title="Le cockpit d'exploitation se construit">
        <p>Les écrans arrivent step par step : chacun avec sa route, son état vide et la permission qui l'ouvre.</p>
      </EmptyState>
    </div>
  )
}
