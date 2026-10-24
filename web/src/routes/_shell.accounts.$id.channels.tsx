import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { accountsKey, useAccount } from '../accounts'
import { gw } from '../gw'
import { Card, Confirmation, Switch, useSubmission, useToast } from '../ui'

export const Route = createFileRoute('/_shell/accounts/$id/channels')({ component: AccountChannels })

const lastChannel = 'Au moins un canal reste activé.'
const disconnects = 'Le changement déconnecte les binds ouverts du compte.'

type Channel = 'smpp' | 'rest'
type Operation = 'query_sm' | 'cancel_sm'

function AccountChannels() {
  const { account, frozen } = useAccount()
  const queryClient = useQueryClient()
  const notify = useToast()
  const { submit, refusal, fail } = useSubmission()
  const [busy, setBusy] = useState(false)
  const [asked, setAsked] = useState<{ operation: Operation; on: boolean }>()
  const key = [...accountsKey, account.id]
  const enabled = { smpp: account.smpp_enabled, rest: account.rest_enabled }

  async function turn(channel: Channel, on: boolean) {
    setBusy(true)
    const outcome = await submit(() =>
      gw.PATCH('/admin/accounts/{id}/channels', {
        params: { path: { id: account.id } },
        body: {
          smpp_enabled: channel === 'smpp' ? on : account.smpp_enabled,
          rest_enabled: channel === 'rest' ? on : account.rest_enabled,
        },
      }),
    )
    setBusy(false)
    if (outcome.failure === undefined) {
      queryClient.setQueryData(key, outcome.data)
      void queryClient.invalidateQueries({ queryKey: accountsKey })
      notify(`Canal ${channel.toUpperCase()} ${on ? 'activé' : 'désactivé'}.`)
      return
    }
    if (outcome.response?.status === 422) {
      fail(lastChannel)
    }
    void queryClient.invalidateQueries({ queryKey: key })
  }

  const channel = (name: Channel, about: string) => {
    const other = name === 'smpp' ? enabled.rest : enabled.smpp
    return (
      <Switch
        label={`Canal ${name.toUpperCase()}`}
        about={about}
        checked={enabled[name]}
        onChange={(on) => void turn(name, on)}
        busy={busy}
        blockedReason={frozen ?? (!other ? lastChannel : undefined)}
      />
    )
  }

  const operation = (name: Operation, on: boolean) => (
    <Switch
      label={
        <>
          <span className="ui-mono">{name}</span> autorisé
        </>
      }
      about={disconnects}
      checked={on}
      onChange={(next) => setAsked({ operation: name, on: next })}
      blockedReason={frozen}
    />
  )

  return (
    <>
      <div className="ui-cards">
        <Card title="Canaux" aside={<span className="ui-why">s’appliquent immédiatement</span>}>
          {refusal}
          {channel('smpp', 'Les binds SMPP de ce compte ne sont acceptés que si ce canal est activé.')}
          {channel('rest', 'La clé d’API de ce compte n’est acceptée que si ce canal est activé.')}
        </Card>
        <Card title="Opérations SMPP" aside={<span className="ui-why">confirmées : elles se lisent au bind</span>}>
          {operation('query_sm', account.query_sm_enabled)}
          {operation('cancel_sm', account.cancel_sm_enabled)}
        </Card>
      </div>
      <p className="ui-why">Chaque bascule est enregistrée dans le journal d’audit.</p>
      {asked && (
        <Confirmation
          title={`${asked.on ? 'Autoriser' : 'Interdire'} ${asked.operation}`}
          action={asked.on ? 'Autoriser' : 'Interdire'}
          invalidate={accountsKey}
          onClose={() => setAsked(undefined)}
          onDone={() => {
            notify(`${asked.operation} ${asked.on ? 'autorisé' : 'interdit'}.`)
            setAsked(undefined)
          }}
          call={() =>
            gw.PATCH('/admin/accounts/{id}/smpp-ops', {
              params: { path: { id: account.id } },
              body: { [`${asked.operation}_enabled`]: asked.on },
            })
          }
        >
          <p>Les drapeaux se lisent au bind : les binds ouverts du compte sont déconnectés.</p>
        </Confirmation>
      )}
    </>
  )
}
