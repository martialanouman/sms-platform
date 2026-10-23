import { useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useId, useState } from 'react'
import type { components } from './admin.gen'
import { customerStatus, customersKey } from './customers'
import { gw } from './gw'
import { Button, Field, Modal, useSubmission } from './ui'

export type Account = components['schemas']['Account']
type Status = Account['status']

export const accountsKey = [...customersKey, 'accounts']

const rank: Record<Status, number> = { active: 0, suspended: 1, closed: 2 }
const under: Record<Status, string> = { active: '', suspended: ' (client suspendu)', closed: ' (client clos)' }

export const effective = (status: Status, customer: Status) =>
  customerStatus[status] + (rank[customer] > rank[status] ? under[customer] : '')

export const nameTaken = 'Un compte de ce client porte déjà ce nom.'

export function AccountForm({
  customerId,
  account,
  onClose,
}: {
  customerId: string
  account?: Account
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const [name, setName] = useState(account?.name ?? '')
  const form = useId()

  async function save(event: FormEvent) {
    event.preventDefault()
    const outcome = account
      ? await submit(() => gw.PATCH('/admin/accounts/{id}', { params: { path: { id: account.id } }, body: { name } }))
      : await submit(() => gw.POST('/admin/accounts', { body: { customer_id: customerId, name } }))
    const said = { 409: nameTaken, 422: 'Ce client est clos.' }[outcome.response?.status ?? 0]
    if (said) {
      fail(said)
      return
    }
    if (outcome.failure === undefined) {
      void queryClient.invalidateQueries({ queryKey: accountsKey })
      onClose()
    }
  }

  return (
    <Modal
      title={account ? `Renommer le compte ${account.name}` : 'Nouveau compte'}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant="primary" type="submit" form={form} loading={waiting}>
            {account ? 'Enregistrer' : 'Créer le compte'}
          </Button>
        </>
      }
    >
      <form className="form" id={form} onSubmit={save}>
        {refusal}
        <Field
          label="Nom"
          required
          maxLength={200}
          autoComplete="off"
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-describedby={describedBy}
        />
        {!account && (
          <p>Canaux, sessions et opérations SMPP prennent leur valeur par défaut ; réglez-les sur sa fiche.</p>
        )}
      </form>
    </Modal>
  )
}
