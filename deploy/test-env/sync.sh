#!/usr/bin/env bash
# Installé hors du dépôt par bootstrap.sh : le dépôt change de tag sous lui, et un tag peut ne pas le contenir.
set -euo pipefail
marker=/var/lib/sms-deploy/deployed
cd /opt/sms-platform
latest=$(git ls-remote --tags --refs origin 'v*' | awk -F/ '{print $3}' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n1 || true)
[ -n "$latest" ] || exit 0
[ "$latest" != "$(cat "$marker" 2>/dev/null || true)" ] || exit 0
git fetch -q --tags origin
git merge-base --is-ancestor "$latest" origin/main || { echo "sync.sh: $latest n'est pas sur main" >&2; exit 1; }
git cat-file -e "$latest:deploy/test-env/deploy.sh" 2>/dev/null || { echo "sync.sh: $latest ne sait pas se déployer" >&2; exit 1; }
git checkout -q --detach "$latest"
images=$(grep -hoE 'ghcr\.io/martialanouman/sms-platform/[a-z-]+' deploy/k8s/*.yaml | sort -u) || { echo "sync.sh: $latest n'a aucune image" >&2; exit 1; }
for image in $images; do
	k3s crictl pull "$image:$latest" >/dev/null || { echo "sync.sh: $image:$latest pas tirable" >&2; exit 1; }
done
deploy/test-env/deploy.sh "$latest"
mkdir -p "$(dirname "$marker")"
echo "$latest" >"$marker"
bash -n deploy/test-env/sync.sh && install -m 755 deploy/test-env/sync.sh /usr/local/libexec/sms-deploy-sync
