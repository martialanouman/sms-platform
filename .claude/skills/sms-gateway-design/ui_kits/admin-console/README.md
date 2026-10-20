# UI kit — SMS Gateway · console d'exploitation

Recréation haute fidélité de la console : le **fond fonctionnel** vient de `uploads/specification-technique-tableau-de-bord.md` (section par section), la **forme visuelle** de `uploads/Design gp-gateway.pdf` — charte v1.0, thème sombre, accent teal unique, boutons en contour, `link_status` en point et `breaker_state` en pilule.

## Fichiers

| Fichier | Rôle |
|---|---|
| `index.html` | Parcours cliquable complet : login + MFA → trafic → CDR → compte SMPP → routes → facturation |
| `screen-traffic.html` | Écran de trafic seul (point de départ) |
| `screen-cdr.html` | CDR Explorer seul (point de départ) |
| `AppShell.jsx` | Rail de navigation, barre supérieure, pile de toasts, `Chart`, `Toolbar`, `Page` |
| `LoginScreen.jsx` | Email + mot de passe → challenge MFA (passkey / TOTP), §6.9 |
| `TrafficScreen.jsx` | §6.3 — compteurs WS, séries MT/MO, ventilation connecteur/client, `link_status` vs `breaker_state` |
| `CdrExplorerScreen.jsx` | §6.4 + §6.12 — recherche par curseur, panneau de détail, cascade de trace, corps gardé par `content:read`, export gouverné |
| `AccountScreen.jsx` | §6.14 + §6.5 — deux identifiants masqués, rotation avec grâce, quotas, écart `max_sessions`, webhooks |
| `RoutesScreen.jsx` | §6.1 + §6.2 + §6.7 — table par priorité, éditeur de route, santé des scripts, simulateur avec bandeau de priorité |
| `BillingScreen.jsx` | §6.11 — solde MT vs compteur MO, ventilation par compte, grand livre, `balance_scope` inerte |
| `mock.js` | Données factices (formes de l'API BFF §5.1, valeurs inventées) |

## Interactions réellement câblées

- Login → MFA (passkey ou TOTP) → console.
- Navigation entre les cinq écrans ; les entrées non maquettées affichent un état vide explicite plutôt qu'un écran inventé.
- CDR : sélection de ligne → panneau, onglets Détail / Trace / Corps, révélation auditée du corps, modale d'export gouverné.
- Compte SMPP : onglets, modale de rotation (fenêtre de grâce → avertissement variable), déconnexion forcée d'une session, écart quota/binds vivants.
- Routes : réordonnancement par priorité, édition, simulateur montrant la précédence numéro exact > script > déclaratif.
- Facturation : recharge MT (MO désactivé), changement de `balance_scope` inerte avec explication.
- Toast Alertmanager poussé ~3,5 s après connexion (flux WS simulé), barre latérale colorée selon la sévérité.
- Fenêtre temporelle en contrôle segmenté (5 min WS / 1 h / 24 h REST), comme dans la charte §05.

## Écarts assumés

- Les graphiques sont des tracés SVG cosmétiques suivant la charte §07 (aire teal + ligne, ligne bleue secondaire, grille discrète) ; la spécification prévoit visx/Recharts en production.
- L'éditeur Monaco (§6.2) n'est pas embarqué : la page Routes montre la table de santé des scripts et le point d'entrée.
- Anti-spam, désabonnements, numéros entrants, contenu/RGPD, opérateurs/rôles et journal d'audit ne sont pas maquettés — la spécification les décrit mais ne fournit aucune référence visuelle.
