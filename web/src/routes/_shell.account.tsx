import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { api, json } from '../api'
import type { components } from '../api.gen'
import { CopyButton, PasskeySection, TotpReplacement } from '../factor'
import { meQuery, useShellMe } from '../session'
import { Button, Modal, PageHeader, useSubmission } from '../ui'

export const Route = createFileRoute('/_shell/account')({ component: Account })

function Account() {
  const me = useShellMe()
  const queryClient = useQueryClient()
  const refreshMe = () => queryClient.invalidateQueries({ queryKey: meQuery.queryKey })

  // debt: #157
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
          <PersonalToken allowed={me.data.permissions.includes('gateway:personal_token')} />
        </>
      )}
    </section>
  )
}

const expiry = new Intl.DateTimeFormat('fr-FR', { hour: '2-digit', minute: '2-digit' })

function PersonalToken({ allowed }: { allowed: boolean }) {
  const id = useId()
  const { submit, waiting, refusal } = useSubmission()
  const [issued, setIssued] = useState<components['schemas']['GatewayToken']>()

  async function issue() {
    const outcome = await submit(() => api.POST('/gateway-token', { headers: json }))
    if (outcome.failure === undefined) {
      setIssued(outcome.data)
    }
  }

  return (
    <section className="auth__form" aria-labelledby={id}>
      <h2 className="auth__subtitle" id={id}>
        Jeton d’API personnel
      </h2>
      <p className="auth__aside">
        Le jeton vaut une heure. Il porte les scopes de vos rôles, moins content:read, content:erase et gdpr:erase. Sa
        délivrance est enregistrée dans le journal d’audit.
      </p>
      {refusal}
      <div>
        <Button
          blockedReason={allowed ? undefined : 'La délivrance d’un jeton personnel demande gateway:personal_token.'}
          loading={waiting}
          onClick={() => void issue()}
        >
          Délivrer
        </Button>
      </div>
      {issued && (
        <Modal
          title="Jeton d’API personnel"
          onClose={() => setIssued(undefined)}
          footer={<Button onClick={() => setIssued(undefined)}>Fermer</Button>}
        >
          <p>
            Ce jeton ne sera plus jamais affiché. Il expire à {expiry.format(new Date(issued.expiresAt))} et ouvre :{' '}
            {issued.scopes.join(', ')}.
          </p>
          <code className="auth__secret">{issued.token}</code>
          <CopyButton label="Copier le jeton" value={issued.token} />
        </Modal>
      )}
    </section>
  )
}
