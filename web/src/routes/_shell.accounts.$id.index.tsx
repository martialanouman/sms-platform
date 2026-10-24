import { createFileRoute, Navigate } from '@tanstack/react-router'
import { useShellMe } from '../session'

export const Route = createFileRoute('/_shell/accounts/$id/')({ component: AccountIndex })

function AccountIndex() {
  const { id } = Route.useParams()
  const me = useShellMe()
  if (!me.data) {
    return null
  }
  return (
    <Navigate
      to={me.data.permissions.includes('credentials:read') ? '/accounts/$id/credentials' : '/accounts/$id/channels'}
      params={{ id }}
      replace
    />
  )
}
