# Step 012f — Retours de l'essai humain

Plan : `docs/plan/composants.md` §012, PR de correction après l'essai humain de 012e. Arbitrages antérieurs : 1 à 213.
Spec : `docs/spec/07-plan-de-controle.md` §7.4 (liste des scopes générée, consommée par les types de la SPA),
`docs/spec/08-dashboard.md` §8.5. Charte : `.claude/skills/sms-gateway-design` (README « Language », « Interaction
states » ; `components/forms/` : `Field`, `Input`, `pl-input--invalid`, `pl-field__error`). Mode : PR.

## Contexte

Le 2026-10-21, l'humain a essayé 012e sur `contabo169`, et a fait trois retours :

1. L'anneau de focus d'un champ n'a pas l'arrondi du champ : l'anneau global de `base.css` (`--focus-ring`) entoure
   l'input à 2 px de distance.
2. Un champ obligatoire vide ouvre la bulle native du navigateur (« Please fill in this field. ») : en anglais, et
   sans couleur d'erreur.
3. Le sélecteur de permissions montre des clés brutes : groupes et permissions doivent se lire en français.

**Terminé quand** : dans Chromium, sur l'environnement de test, le focus d'un champ épouse son arrondi ; soumettre
« Nouvel opérateur » vide colore les champs en rouge et dit en français, sous chacun, ce qui manque, sans bulle
native ; le sélecteur et la vue d'un rôle se lisent en français, l'identifiant restant visible ; axe ne relève
aucune violation.

## Arbitrages

Charte et spec, 2026-10-21.

214. **Focus d'un champ** : celui du kit (`.pl-input:focus`), bordure `--border-focus` et halo de 3 px
     `--tint-teal` collé à la bordure, à la place de l'anneau global pour `.ui-input` seulement.
215. **Erreur d'un champ** : celle du kit (`pl-input--invalid`, `pl-field__error`). La validation reste celle du
     navigateur (`required`, `type="email"`, `maxLength`) ; `Field` écoute `invalid`, annule la bulle, pose
     `aria-invalid` et un message français sous le champ, relié par `aria-describedby`, et donne le focus au premier
     champ invalide. Le message s'efface quand la saisie redevient valide. Les formulaires d'authentification
     perdent `noValidate` : tous les champs suivent la même règle.
216. **Messages** : « Ce champ est obligatoire. » ; « Adresse e-mail invalide. » ; sinon « Valeur invalide. ». Jamais
     `validationMessage` : il parle la langue du navigateur.
217. **Libellés des permissions** : un libellé français tient lieu de l'identifiant sans le remplacer (charte,
     « Language ») : légende du groupe en français, option en français, clé en mono à côté. Le libellé d'une option
     est « Lire » pour `:read`, « Modifier » pour `:write`, et un libellé propre pour les autres actions.
218. **Exhaustivité** : `tools/openapi` écrit aussi `web/src/scopes.gen.ts` (§7.4 : la liste générée nourrit les
     types de la SPA) ; les libellés sont des `Record` sur ces types, et `tsc` refuse un scope ou une ressource
     sans libellé. Une clé inconnue du client (serveur plus récent) s'affiche brute.
219. **Portée** : sélecteur de l'éditeur et vue « Voir les permissions ». La liste des scopes du jeton personnel
     garde les clés : c'est ce qu'on lit dans les journaux de la passerelle.

## Design arrêté

- **`internal/openapi`**, **`tools/openapi`** : sortie TypeScript (`-ts`), triée, `as const`, et son test ;
  `doc.go` : la ligne `go:generate`.
- **`web/src/scopes.gen.ts`** (généré) ; exclu de Biome comme les autres fichiers générés.
- **`web/src/permissions.ts`** : libellés des ressources et des actions, fonction de libellé.
- **`web/src/ui.tsx`** : `Field` (validation française, `aria-invalid`, message, focus).
- **`web/src/routes/_shell.roles.tsx`** : sélecteur et vue en français.
- **Formulaires** : `noValidate` retiré de `login.tsx`, `access.tsx`, `factor.tsx`.
- **`web/src/styles/app.css`** : focus et erreur des champs, libellés du sélecteur, en jetons.
- **Tests** (Go : sortie TypeScript ; Vitest, `StrictMode`, axe) : message et couleur d'erreur sans bulle, focus au
  premier invalide, effacement, libellés français et clé visible, clé inconnue brute. Mutation manuelle, consignée.
- **Preuve sur `contabo169`**, dans Chromium : le parcours du « Terminé quand », captures, axe.
