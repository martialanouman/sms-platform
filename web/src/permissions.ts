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
  'dead-letters': 'Lettres mortes',
  'exact-routes': 'Routes par numéro exact',
  gateway: 'Passerelle',
  gdpr: 'RGPD',
  'inbound-numbers': 'Numéros entrants',
  jobs: 'Tâches',
  msisdn: 'Numéros d’abonnés',
  'opt-out-keywords': 'Mots-clés d’opt-out',
  operators: 'Opérateurs',
  platform: 'Plateforme',
  roles: 'Rôles',
  routes: 'Routes',
  'routing-scripts': 'Scripts de routage',
  'sender-ids': 'Sender ID',
  'sender-rewrite': 'Réécriture du sender ID',
  sessions: 'Sessions du tableau de bord',
  'smpp-sessions': 'Sessions SMPP',
  stream: 'Flux temps réel',
  suppressions: 'Opt-out',
  traffic: 'Trafic',
}

const actions: Record<Special, string> & Partial<Record<Scope, string>> = {
  'billing:topup': 'Recharger et fixer le crédit',
  'cdr:export_bulk': 'Exporter en masse',
  'content:erase': 'Effacer',
  'content:write': 'Régler le stockage et la rétention',
  'gateway:personal_token': 'Se délivrer un jeton personnel',
  'gdpr:erase': 'Effacer les données d’un abonné',
  'msisdn:reveal': 'Démasquer',
  'routing-scripts:publish': 'Publier',
  'sessions:write': 'Fermer de force',
  'smpp-sessions:write': 'Déconnecter de force',
  'suppressions:delete': 'Lever un opt-out',
}

export function resourceLabel(resource: string) {
  return (resources as Record<string, string | undefined>)[resource] ?? resource
}

export function actionLabel(key: string) {
  const known = (actions as Record<string, string | undefined>)[key]
  if (known) {
    return known
  }
  return { read: 'Lire', write: 'Modifier' }[key.split(':')[1] ?? '']
}
