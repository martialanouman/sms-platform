# Step 007d-2b — Livraison : déploiement par tag, retour arrière

Plan : `docs/plan/socle.md` §007, dernière des sept PR. Spec : `docs/spec/10-exploitation.md` §10.2 (une version,
déploiement par tag de release, ordre `migrate` → `catalog-apply` → services, environnement de test déployé à
chaque release). Précédents : `docs/steps/007d-1-briques.md` (la procédure est la spécification du
déploiement), `docs/steps/007d-2a-tag-et-release.md` (tags, images `:vX.Y.Z`). Règles :
`.claude/rules/ci-deploy.md`, `.claude/rules/tests.md`. Mode : PR.

## Contexte

- 007d-2a pose `vX.Y.Z` et pousse les images `ghcr.io/martialanouman/sms-platform/<cmd>:vX.Y.Z` à chaque merge qui
  change le binaire. Les paquets naissent privés et se rendent publics une fois, à la main.
- 007d-1 a joué à la main, sur `contabo169`, la séquence : opérateurs → CA → `secrets.sh` → briques →
  NetworkPolicy → `migrate` → `catalog-apply`. Les manifests n'ont pas de tag d'image ; les Jobs n'ont pas de borne
  de temps.
- Décisions humaines du 2026-10-19 : la VM tire ; aucun secret de déploiement dans GitHub ; déploiement par tag de
  release ; retour arrière par `git revert`, qui produit une release.

**Terminé quand** (critère de fin de la 007) : une release déploie l'environnement de test sans geste manuel, et
`catalog-apply` y crée les topics et leurs ACL.

## Arbitrages

Fable a tranché le 2026-10-19 (verdict sur la 007d-2, intégrant la décision humaine du déploiement par tag) :

- **Tirage** : `deploy/test-env/sync.sh` sous un timer systemd (`OnUnitInactiveSec=1min`, `Type=oneshot`) : le
  dernier tag `v*` par `git ls-remote --tags --refs` et tri de version ; égal au marqueur
  `/var/lib/sms-deploy/deployed` → rien ; sinon `git fetch --tags`, `git checkout` du tag, chaque image du tag
  pré-tirée (`k3s crictl pull`, une image absente remet au tick suivant : couvre la course entre tag et
  images), `deploy.sh <tag>`, puis le marqueur, sur succès seulement. Pas d'état « tag en échec » : systemd
  sérialise (le timer n'arme pas une unité active) ; un échec transitoire se répare au tick suivant, un échec
  déterministe se rejoue visiblement (`systemctl status`) jusqu'au `revert`. Plafond nommé : un marqueur d'échec
  si le bruit coûte. Pas d'épingle locale d'un tag : `git revert` est la voie décidée.
- **Amorçage** : `deploy/test-env/bootstrap.sh`, lancé une fois sur une VM nue : k3s épinglé, `git clone` du dépôt
  public, unités systemd, `systemctl enable --now`. La VM ne connaît que l'URL publique du dépôt : ni adresse, ni
  secret, ni kubeconfig dans le dépôt.
- **Tag des images** : `deploy.sh <tag>` pose `:<tag>` sur chaque ligne `image:
  ghcr.io/martialanouman/sms-platform/…` à l'application (la garde assure la forme sans tag) ; le dépôt garde des
  manifests sans tag.
- **Jobs bornés** : `activeDeadlineSeconds` — `migrate` 300 (une minute de `pgdial`, plus les migrations),
  `catalog-apply` 900 (trois essais) ; la garde l'exige de chaque Job. `deploy.sh` supprime un Job puis le
  recrée (un Job est immuable), attend `complete` (borné), et `set -euo pipefail` arrête la séquence au premier
  rouge : les services, à venir après les Jobs, restent à l'état précédent.
- **Séquence** : `deploy.sh` rejoue la procédure de 007d-1, idempotente par `apply`.
- **Preuve** : avant merge, sur `contabo169`, `bootstrap.sh` depuis la table rase puis `deploy.sh v0.1.0` (les vraies
  images de 007d-2a) ; après merge, ce `feat` produit `v0.2.0` et le premier déploiement automatique, consigné
  dans une PR `docs(steps)` qui prouve aussi, en ne publiant pas, qu'un `docs` ne déploie rien.

Fait de la preuve (2026-10-19) : le service lançait `sync.sh` depuis le dépôt, que `sync.sh` bascule sur le tag ;
au premier tick, `v0.1.0`, qui ne le contient pas, l'a fait disparaître (« Unable to locate executable ») et le
timer est resté mort, incapable de voir un tag plus récent. Correctif : `bootstrap.sh` installe `sync.sh` hors du
dépôt (`/usr/local/libexec/sms-deploy-sync`), que le service lance ; après chaque déploiement réussi, `sync.sh` se
remplace par la version du tag ; un tag sans `deploy.sh` est reconnu avant la bascule
(`git cat-file -e`), qui laisse alors l'arbre en place.

Fable a tranché les majeurs de la revue (2026-10-19) :

- **Un tag hors de `main` est refusé** : `sync.sh` exige un tag ancêtre de `origin/main`
  (`git merge-base --is-ancestor`), car tout tag de la release l'est ; sans lui, un tag poussé sur n'importe
  quel commit ferait exécuter son `deploy.sh` en root. Le ruleset de tags reste le « devra » de `ci-deploy.md`.
- **Tout refus sort en 1**, le silence est « rien à faire » : un tag hors de `main`, sans `deploy.sh`, sans image
  ou dont une image n'est pas tirable (un paquet privé est le cas courant) se lit dans `systemctl status`, et
  `OnUnitInactiveSec` réarme après un échec.
- **Auto-remplacement gardé**, derrière `bash -n` : une copie qui ne se parse pas n'est pas installée. Un défaut
  de logique se répare par la reprise ci-dessous ; les unités ne changent qu'avec elle.
- **`TimeoutStartSec=3h`**, au-dessus de la somme des bornes de `deploy.sh` (environ 2 h 35) ; il borne seul
  `ls-remote`, `fetch` et `crictl pull`.
- **La garde** exige la forme brute `image: ghcr.io/martialanouman/sms-platform/<cmd>` (la seule que le `sed`
  tague : ni guillemets ni commentaire) et chaque `cmd/` une fois dans la boucle des Jobs de `deploy.sh`.
- **Rejeu de `catalog-apply`** (risque de 007d-1) : mesuré par la preuve, pas d'issue.
- **Retour arrière** : prouvé par pièces, `revert` → patch (table de `Next`, 007d-2a), et tout tag plus récent
  se déploie (ici) ; bout en bout au premier `revert` réel sur `main`. Un `revert` de cette step produit un tag
  sans `deploy.sh`, que `sync.sh` refuse.

## Design arrêté

- `deploy/k8s/*.yaml` : `activeDeadlineSeconds` (300 pour `migrate`, 900 pour `catalog-apply`).
- `deploy/guard_test.go` : chaque Job porte `activeDeadlineSeconds` positif ; cas faussé.
- `deploy/test-env/deploy.sh <tag>` : la séquence de 007d-1, images à `:<tag>`, Jobs recréés et attendus.
- `deploy/test-env/sync.sh` : le tirage ci-dessus, lancé depuis sa copie `/usr/local/libexec/sms-deploy-sync`,
  qu'il remplace après chaque succès si elle se parse ; un tag ancêtre de `origin/main` seulement ; tout refus
  en 1. `deploy/test-env/sms-deploy.service` (`TimeoutStartSec=3h`) et `.timer` (`OnBootSec=1min`,
  `OnUnitInactiveSec=1min`).
- `deploy/test-env/bootstrap.sh` : l'amorçage ci-dessus, rejouable (`enable`, puis `restart` du timer).
- `deploy/guard_test.go` : forme brute de l'image ; chaque `cmd/` une fois dans `for job in … ; do` de
  `deploy.sh`.
- `.claude/rules/ci-deploy.md` : un `cmd/` neuf entre dans `deploy.sh` (ordre) avec son manifest.
- Preuve : la procédure ci-dessus, consignée avant la PR.

## Reprise

Une copie de `sync.sh` qui tire mal, ou des unités à changer : en root sur la VM,
`git -C /opt/sms-platform fetch --tags && git -C /opt/sms-platform checkout vX.Y.Z && bash
/opt/sms-platform/deploy/test-env/bootstrap.sh`, `vX.Y.Z` étant un tag qui contient `deploy/test-env`.
