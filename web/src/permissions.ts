import type { Scope } from './scopes.gen'

type Resource = Scope extends `${infer R}:${string}` ? R : never
type Special = Exclude<Scope, `${string}:read` | `${string}:write`>

const resources: Record<Resource, string> = {
  accounts: 'Comptes',
  antispam: 'Anti-spam',
  audit: 'Journal d’audit',
  billing: 'Facturation',
  cdr: 'CDR',
  connectors: 'Connecteurs',
  content: 'Corps des messages',
  credentials: 'Identifiants',
  'customer-groups': 'Groupes de clients',
  customers: 'Clients',
  'dead-letters': 'Dead-letters',
  'exact-routes': 'Routes par numéro exact',
  gateway: 'Jeton personnel',
  gdpr: 'RGPD',
  'inbound-numbers': 'Numéros entrants',
  jobs: 'Jobs',
  msisdn: 'MSISDN',
  'opt-out-keywords': 'Mots-clés d’opt-out',
  operators: 'Opérateurs',
  platform: 'Plateforme',
  roles: 'Rôles',
  routes: 'Routes',
  'routing-scripts': 'Scripts de routage',
  'sender-ids': 'Sender IDs',
  'sender-rewrite': 'Réécriture du sender ID',
  sessions: 'Sessions des opérateurs',
  'smpp-sessions': 'Sessions SMPP',
  stream: 'Flux temps réel',
  suppressions: 'Opt-out',
  traffic: 'Trafic',
}

const actions: Record<Special, string> & Partial<Record<Scope, string>> = {
  'billing:topup': 'Recharger, ajuster et fixer les conditions de crédit',
  'cdr:export_bulk': 'Exporter en masse',
  'content:erase': 'Effacer',
  'content:write': 'Régler le stockage et la rétention',
  'gateway:personal_token': 'Se délivrer',
  'gdpr:erase': 'Effacer les données d’un abonné',
  'msisdn:reveal': 'Démasquer',
  'routing-scripts:publish': 'Publier',
  'sessions:write': 'Fermer de force',
  'smpp-sessions:write': 'Déconnecter de force',
  'suppressions:delete': 'Lever un opt-out',
}

const own = (labels: object, key: string) =>
  Object.hasOwn(labels, key) ? (labels as Record<string, string>)[key] : undefined

export function permissionLabel(key: string) {
  const [resource = '', action = ''] = key.split(':')
  const verb = own(actions, key) ?? own({ read: 'Lire', write: 'Modifier' }, action)
  return { resource: own(resources, resource), action: verb }
}
