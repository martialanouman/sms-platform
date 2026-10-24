import { createFileRoute } from '@tanstack/react-router'
import { useAccount } from '../accounts'
import { Webhooks } from '../webhooks'

export const Route = createFileRoute('/_shell/accounts/$id/webhooks')({ component: AccountWebhooks })

function AccountWebhooks() {
  return <Webhooks account={useAccount().account} />
}
