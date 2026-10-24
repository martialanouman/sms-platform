import type { Scope } from './scopes.gen'

type Resource = Scope extends `${infer R}:${string}` ? R : never

const resources: Record<Resource, string> = {
  customers: 'Clients',
  accounts: 'Comptes',
  credentials: 'Identifiants',
  'customer-groups': 'Groupes de clients',
  routes: 'Routes',
  'exact-routes': 'Numéros exacts',
  'routing-scripts': 'Scripts de routage',
  connectors: 'Connecteurs',
  'smpp-sessions': 'Sessions SMPP',
  'sender-ids': 'Sender IDs',
  'sender-rewrite': 'Réécriture',
  'inbound-numbers': 'Numéros entrants',
  traffic: 'Trafic',
  stream: 'Temps réel',
  cdr: 'CDR',
  'dead-letters': 'Messages en échec',
  jobs: 'Tâches',
  antispam: 'Anti-spam',
  'opt-out-keywords': 'Mots d’opt-out',
  suppressions: 'Désabonnements',
  content: 'Corps des messages',
  msisdn: 'Numéros des abonnés',
  gdpr: 'Données personnelles',
  billing: 'Facturation',
  platform: 'Plateforme',
  audit: 'Journal d’audit',
  operators: 'Opérateurs',
  roles: 'Rôles',
  sessions: 'Sessions du tableau de bord',
  gateway: 'Jeton personnel',
}

const verbs = {
  read: 'Lire',
  write: 'Modifier',
  erase: 'Effacer',
  delete: 'Supprimer',
  publish: 'Publier',
  reveal: 'Révéler',
}

const exceptions: Record<Exclude<Scope, `${string}:${keyof typeof verbs}`>, string> = {
  'billing:topup': 'Recharger, ajuster et fixer les conditions de crédit',
  'cdr:export_bulk': 'Exporter en masse',
  'gateway:personal_token': 'Émettre',
}

export const resourceOrder: readonly string[] = Object.keys(resources)

const own = (labels: object, key: string) =>
  Object.hasOwn(labels, key) ? (labels as Record<string, string>)[key] : undefined

export function permissionLabel(key: string) {
  const [resource = '', action = ''] = key.split(':')
  return { resource: own(resources, resource), action: own(exceptions, key) ?? own(verbs, action) }
}
