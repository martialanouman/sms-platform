import { createFileRoute } from '@tanstack/react-router'
import { useAccount } from '../accounts'
import { Credentials } from '../credentials'

export const Route = createFileRoute('/_shell/accounts/$id/credentials')({ component: AccountCredentials })

function AccountCredentials() {
  return <Credentials account={useAccount().account} />
}
