import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { PasskeySection, TotpReplacement } from '../factor'
import { meQuery, useShellMe } from '../session'
import { PageHeader } from '../ui'

export const Route = createFileRoute('/_shell/account')({ component: Account })

function Account() {
  const me = useShellMe()
  const queryClient = useQueryClient()
  const refreshMe = () => queryClient.invalidateQueries({ queryKey: meQuery.queryKey })

  return (
    <section className="page">
      <PageHeader title="Mon compte" />
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
