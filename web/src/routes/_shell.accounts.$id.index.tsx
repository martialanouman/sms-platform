import { createFileRoute, Navigate } from '@tanstack/react-router'
import { useShellMe } from '../session'

export const Route = createFileRoute('/_shell/accounts/$id/')({ component: AccountIndex })

function AccountIndex() {
  const { id } = Route.useParams()
  const held = useShellMe().data?.permissions ?? []
  return (
    <Navigate
      to={held.includes('credentials:read') ? '/accounts/$id/credentials' : '/accounts/$id/channels'}
      params={{ id }}
      replace
    />
  )
}
