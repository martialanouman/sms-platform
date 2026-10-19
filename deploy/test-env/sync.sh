#!/usr/bin/env bash
# Installé hors du dépôt par bootstrap.sh : le dépôt change de tag sous lui, et un tag peut ne pas le contenir.
set -euo pipefail
marker=/var/lib/sms-deploy/deployed
cd /opt/sms-platform
latest=$(git ls-remote --tags --refs origin 'v*' | awk -F/ '{print $3}' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n1 || true)
[ -n "$latest" ] || exit 0
[ "$latest" != "$(cat "$marker" 2>/dev/null || true)" ] || exit 0
git fetch -q --tags origin
git cat-file -e "$latest:deploy/test-env/deploy.sh" 2>/dev/null || { echo "sync.sh: $latest ne sait pas se déployer"; exit 0; }
git checkout -q --detach "$latest"
while read -r image; do
	k3s crictl pull "$image:$latest" >/dev/null || { echo "sync.sh: $image:$latest pas encore tirable"; exit 0; }
done < <(grep -hoE 'ghcr\.io/martialanouman/sms-platform/[a-z-]+' deploy/k8s/*.yaml | sort -u)
deploy/test-env/deploy.sh "$latest"
mkdir -p "$(dirname "$marker")"
echo "$latest" >"$marker"
install -m 755 deploy/test-env/sync.sh /usr/local/libexec/sms-deploy-sync
