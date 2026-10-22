import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { api, json } from '../api'
import type { components } from '../api.gen'
import { CopyButton, PasskeySection, TotpReplacement } from '../factor'
import { PermissionPicker } from '../picker'
import { adminScopes } from '../scopes.gen'
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
          <PersonalToken held={me.data.permissions} />
        </>
      )}
    </section>
  )
}

const expiry = new Intl.DateTimeFormat('fr-FR', { hour: '2-digit', minute: '2-digit' })

const personalExcluded = ['content:read', 'content:erase', 'gdpr:erase']

function PersonalToken({ held }: { held: string[] }) {
  const id = useId()
  const allowed = held.includes('gateway:personal_token')
  const catalog = adminScopes.filter((s) => held.includes(s) && !personalExcluded.includes(s))
  const [chosen, setChosen] = useState<string[]>(catalog)
  const { submit, waiting, refusal } = useSubmission()
  const [issued, setIssued] = useState<components['schemas']['GatewayToken']>()

  async function issue() {
    const scopes = [...chosen].sort() as components['schemas']['GatewayTokenRequest']['scopes']
    const outcome = await submit(() => api.POST('/gateway-token', { headers: json, body: { scopes } }))
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
        Le jeton expire une heure après sa délivrance. Il porte les scopes de vos rôles, moins content:read,
        content:erase et gdpr:erase. Sa délivrance est enregistrée dans le journal d’audit.
      </p>
      {refusal}
      {allowed && catalog.length > 0 && (
        <fieldset className="token-scopes">
          <legend>Scopes du jeton</legend>
          <PermissionPicker keys={catalog} chosen={chosen} onChange={setChosen} />
        </fieldset>
      )}
      <div>
        <Button
          blockedReason={
            !allowed
              ? 'Se délivrer un jeton personnel demande gateway:personal_token, que seul le rôle Propriétaire détient par défaut ; un administrateur peut l’accorder.'
              : chosen.length === 0
                ? 'Cochez au moins un scope.'
                : undefined
          }
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
          footer={
            <>
              <CopyButton label="Copier le secret" value={issued.token} />
              <Button variant="primary" onClick={() => setIssued(undefined)}>
                J’ai copié le secret
              </Button>
            </>
          }
        >
          <p>
            Ce jeton ne sera plus jamais affiché. Il expire à {expiry.format(new Date(issued.expiresAt))} et ouvre :{' '}
            {issued.scopes.join(', ')}.
          </p>
          <code className="auth__secret">{issued.token}</code>
        </Modal>
      )}
    </section>
  )
}
