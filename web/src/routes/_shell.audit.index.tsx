import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_shell/audit/')({
  beforeLoad: () => {
    throw redirect({ to: '/audit/control-plane' })
  },
})
