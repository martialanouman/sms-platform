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
      <EmptyState as="h1" title="Le cockpit d'exploitation se construit">
        <p>
          Les écrans arrivent step par step. Les rôles et le jeton personnel arrivent avec la step 012e ; le trafic en
          direct et les sessions SMPP, avec la step 026.
        </p>
        <p>
          Chaque écran arrive avec sa route, son état vide et la permission qui l'ouvre : aucun lien ne mène nulle part.
        </p>
      </EmptyState>
    </div>
  )
}
