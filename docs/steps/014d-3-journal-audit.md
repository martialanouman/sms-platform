# Step 014d-3 — Journal d'audit

Troisième des quatre PR de 014d (coupe : arbitrage 426). Les arbitrages de toute la 014d sont dans
`docs/steps/014d-1-operateurs.md` (426 à 451) ; cette PR applique 430 (libellés du `Pager`, `noun` tombe), 445 à 447,
448 et 449 pour sa part. Maquette : `docs/steps/014d/maquette/AuditAdmin.dc.html`, `AuditModifs.dc.html`,
`AuditAcces.dc.html`. Spec : `docs/spec/08-dashboard.md` §8.8, §8.9. Mode : PR.

**Terminé quand**, dans le navigateur sur contabo169 : les deux onglets suivent la maquette (sans phrase
d'introduction, filtres Opérateur, Du, Au et « Retirer les filtres » sans bouton « Filtrer », table dans une carte,
pilule Résultat, colonne Détail, pied « N entrées affichées » et Plus récentes / Plus anciennes) ; la modale des
modifications suit la maquette (Date, Opérateur par son nom, Requête ; un bloc par changement, champs changés en
table, inchangés repliés, ligne sur les secrets) ; axe sans violation, aucun `[title]`.

## Design arrêté

- **`web/src/list.tsx`** : `Pager({ search, next, go, count, labels? })` ; un seul libellé « N entrées affichées »
  (« 1 entrée affichée ») ; `labels` vaut Précédent / Suivant par défaut ; `noun` tombe chez tous ses appelants
  (Clients, Comptes, Groupes, les deux journaux, la modale des modifications) et les tests suivent (430). Le `total`
  arrive avec la 014d-4.
- **`web/src/audit.tsx`** : `AuditFilters` sans `form` ni bouton « Filtrer » ; Opérateur (`Combobox`), Du, Au
  (`type="date"`) : chaque changement navigue ; « Retirer les filtres » revient au défaut, figé et expliqué « Aucun
  filtre actif. » quand on y est (446).
- **`_shell.audit.control-plane.tsx`** : table dans une `Card` ; Date (mono), Opérateur (`useOperatorName`, `service:*`
  en mono), Opération (mono), Cible (mono), Résultat en `StatusPill` (445 : `outcome` présent → `granted` ok « lecture
  accordée », `unreadable` warn « illisible », `not_found` warn « introuvable » ; sinon `< 400` ok « code », `< 500`
  warn « code refusé », sinon bad « code erreur »), Détail (« Modifications » ou « aucune » muet) ; pied `Pager`
  Plus récentes / Plus anciennes.
- **`_shell.audit.access.tsx`** : même carte ; Résultat Réussi ok, Échec warn ; « inconnu » muet pour un acteur absent ;
  même pied.
- **`_shell.audit.tsx`** : sans phrase d'introduction (retour humain).
- **Modale des modifications** (447) : reçoit l'entrée ; titre « Modifications de `operation_id` » ; `KeyValue` Date,
  Opérateur (nom), Requête (mono) ; par changement : étiquette Création | Modification | Suppression, table et `pk` en
  mono ; table Champ / Avant / Après des seuls champs changés, `null` → « vide » muet, scalaires en texte, objets en
  `JSON.stringify` ; `details` « N champs inchangés » qui les liste ; ligne « Les secrets et leurs empreintes ne sont
  jamais journalisés : seuls les champs lisibles apparaissent. » ; `Pager` gardé, « N entrées affichées ».
- **Aucun changement de contrat ni de Go.**
- **Tests** : 449 pour la part 014d-3 (table de la pilule Résultat : 200, 201, 409, 500, `granted` ; champs changés
  contre repliés ; « Retirer les filtres » figé au défaut ; libellés du pager) ; mutation consignée ; preuve sur
  contabo169, captures dans `docs/steps/014d-3/`.

## Écarts au design, constatés à l'implémentation

- **`Combobox` prend un `onChange`** (`onValueChange` de Base UI) : sans formulaire ni « Filtrer », le choix d'un
  opérateur doit naviguer de lui-même.
- **Rangée des filtres** : `.audit-filters` gardée plutôt que `.ui-filters` ; les champs de l'audit portent leur
  libellé au-dessus, que `.ui-filters` centrerait contre le bouton.
- **Dates des filtres** : seule une date complète navigue ; une saisie partielle ou vidée ne change rien.
- **Modale des modifications** : elle reçoit l'entrée et son `request_id` ; l'acteur et la date par changement tombent
  (ceux de la requête suffisent). Un champ se compare par son JSON ; la garde « avant et après présents » est tombée,
  la mutation l'ayant montrée sans effet (les clés viennent de l'union des deux côtés).
- **CSS** : `.audit-change__table`, `.audit-change__meta` et `.audit-diff*` supprimées ; `.audit-change__head` ajoutée.
- **Mutations** (toutes tuées, restauration vérifiée par `cmp`) : seuils 400 et 500, branche `outcome`, filtre des
  champs changés, compte et accord des champs inchangés, figé de « Retirer les filtres », libellés du pager par défaut
  et ceux des deux journaux, accord « 1 entrée affichée ».
- **Correctif de revue 1** : une date s'applique en quittant le champ ou sur Entrée, jamais à chaque frappe. Au
  clavier, le « 1 » de « 15 » formait déjà une date complète : le filtre naviguait, la clé du `fieldset` qui
  dépendait des filtres remontait tout, le focus partait et chaque frappe ajoutait une entrée d'historique. Les
  champs sont contrôlés et ne se remontent jamais ; le `Combobox` est contrôlé (`value`, `onChange` requis, sans
  `name`) ; le focus reste en place après le choix d'un opérateur et après « Retirer les filtres ».
- **Correctif de revue 2** : « Retirer les filtres » est figé dès que l'état vaut le défaut (aucun opérateur, Du et Au
  à la date du jour), pas seulement quand l'adresse ne porte aucun filtre.
- **Correctif de revue 3** : une modification sans champ lisible changé dit « Aucun champ lisible n'a changé. » au
  lieu d'une table vide.
- **Correctif de revue 4** : la modale suit la maquette : bouton « Fermer » en pied, Avant en rouge barré, Après en
  vert (`.audit-before`, `.audit-after`, sur `--text-danger` et `--text-success`).
- **Correctif de revue 5** : le nom de table est réellement en mono (la règle de `.audit-change__head h3` écrasait
  `.ui-mono`) ; la modale reçoit l'entrée seule ; l'assertion d'absence du bouton « Filtrer » tombe ; les tests de
  saisie quittent le champ (`user.tab()`).
- **Mutations des correctifs** (toutes tuées, restauration vérifiée par `cmp`) : date appliquée à chaque frappe, figé
  limité à l'absence de clés, retour de la clé du `fieldset`, ligne « aucun champ » retirée, pied sans « Fermer »,
  classe Avant retirée.
