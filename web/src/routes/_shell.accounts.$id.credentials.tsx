import { createFileRoute } from '@tanstack/react-router'
import { Credentials } from '../credentials'

export const Route = createFileRoute('/_shell/accounts/$id/credentials')({ component: Credentials })
