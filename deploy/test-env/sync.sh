#!/usr/bin/env bash
# Le corps est une fonction : bash la lit en entier avant de changer de tag l'arbre où ce fichier vit.
set -euo pipefail

main() {
	local marker=/var/lib/sms-deploy/deployed latest image
	cd /opt/sms-platform
	latest=$(git ls-remote --tags --refs origin 'v*' | awk -F/ '{print $3}' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n1 || true)
	[ -n "$latest" ] || return 0
	[ "$latest" != "$(cat "$marker" 2>/dev/null || true)" ] || return 0
	git fetch -q --tags origin
	git checkout -q --detach "$latest"
	[ -x deploy/test-env/deploy.sh ] || { echo "sync.sh: $latest ne sait pas se déployer"; return 0; }
	while read -r image; do
		k3s crictl pull "$image:$latest" >/dev/null || { echo "sync.sh: $image:$latest pas encore tirable"; return 0; }
	done < <(grep -hoE 'ghcr\.io/martialanouman/sms-platform/[a-z-]+' deploy/k8s/*.yaml | sort -u)
	deploy/test-env/deploy.sh "$latest"
	mkdir -p "$(dirname "$marker")"
	echo "$latest" >"$marker"
}

main "$@"
