import { createFileRoute } from '@tanstack/react-router'
import { Webhooks } from '../webhooks'

export const Route = createFileRoute('/_shell/accounts/$id/webhooks')({ component: Webhooks })
