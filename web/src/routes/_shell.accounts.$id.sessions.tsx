import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useId, useState } from 'react'
import { accountQuery, accountsKey, useAccount } from '../accounts'
import { gw } from '../gw'
import { Button, Card, Choice, Choices, EmptyState, Field, useSubmission, useToast } from '../ui'

export const Route = createFileRoute('/_shell/accounts/$id/sessions')({ component: AccountSessions })

const bindTypes = [
  { value: 'transmitter', label: 'Émetteur' },
  { value: 'receiver', label: 'Récepteur' },
  { value: 'transceiver', label: 'Émetteur-récepteur' },
] as const

function AccountSessions() {
  const { account, frozen } = useAccount()
  const queryClient = useQueryClient()
  const notify = useToast()
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const [sessions, setSessions] = useState(String(account.max_sessions))
  const [types, setTypes] = useState<string[]>(account.allowed_bind_types)
  const hint = useId()

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = await submit(() =>
      gw.PATCH('/admin/accounts/{id}/session-limits', {
        params: { path: { id: account.id } },
        body: {
          max_sessions: Number(sessions),
          allowed_bind_types: bindTypes.map((t) => t.value).filter((t) => types.includes(t)),
        },
      }),
    )
    if (outcome.failure === undefined) {
      queryClient.setQueryData(accountQuery(account.id).queryKey, outcome.data)
      void queryClient.invalidateQueries({ queryKey: accountsKey })
      notify('Limites de session enregistrées.')
    }
  }

  return (
    <div className="ui-cards">
      <Card title="Limites de session">
        <form className="form" onSubmit={save}>
          {refusal}
          <div>
            <Field
              label="max_sessions"
              className="ui-input ui-mono"
              type="number"
              required
              min={0}
              max={2147483647}
              step={1}
              readOnly={frozen !== undefined}
              value={sessions}
              onChange={(e) => setSessions(e.target.value)}
              aria-describedby={hint}
            />
            <p className="ui-why" id={hint}>
              Baisser la limite ne coupe aucun bind ouvert.
            </p>
          </div>
          <div>
            <Choices value={types} onChange={setTypes} label="Types de bind" describedBy={describedBy}>
              {bindTypes.map((t) => (
                <Choice
                  value={t.value}
                  key={t.value}
                  disabled={frozen !== undefined || (types.length === 1 && types.includes(t.value))}
                >
                  <span>
                    {t.label} <span className="ui-mono ui-muted">{t.value}</span>
                  </span>
                </Choice>
              ))}
            </Choices>
            <p className="ui-why">Au moins un type reste autorisé.</p>
          </div>
          <div>
            <Button variant="primary" type="submit" loading={waiting} blockedReason={frozen}>
              Enregistrer
            </Button>
          </div>
          <p className="ui-why">L’action est enregistrée dans le journal d’audit.</p>
        </form>
      </Card>
      <Card
        title="Binds vivants"
        aside={
          <span className="ui-why">
            limite <span className="ui-mono">{account.max_sessions}</span>
          </span>
        }
      >
        <EmptyState title="Sessions en direct indisponibles">
          Les binds ouverts de ce compte s’afficheront ici, avec leur type, leur IP et leur débit, dès que le serveur
          SMPP sera en service. Rien n’est caché : aucun bind n’existe encore.
        </EmptyState>
      </Card>
    </div>
  )
}
