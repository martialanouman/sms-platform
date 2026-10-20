#!/usr/bin/env bash
# Les valeurs passent par l'entrée standard, jamais par les arguments d'un processus ; une valeur vide arrête tout,
# car un Secret présent n'est jamais recréé et une erreur dans une substitution échappe à set -e.
set -euo pipefail
cd "$(dirname "$0")/../.."

exists() { kubectl -n "$1" get secret "$2" >/dev/null 2>&1; }
read_secret() { kubectl -n "$1" get secret "$2" -o "jsonpath={.data.$3}" | base64 -d; }
nonempty() { [ -n "$2" ] || { echo "secrets.sh: $1 est vide" >&2; exit 1; }; }
create() {
	nonempty "$1/$2" "$4"
	printf '%s' "$4" | kubectl -n "$1" create secret generic "$2" "${@:5}" --from-file="$3"=/dev/stdin
}

if ! exists postgres migrator; then
	password=$(openssl rand -hex 32)
	create postgres migrator password "$password" --type=kubernetes.io/basic-auth --from-literal=username=migrator
fi

if ! exists postgres openbao; then
	password=$(openssl rand -hex 32)
	create postgres openbao password "$password"
fi

password=$(read_secret postgres migrator password)
nonempty postgres/migrator "$password"
url="postgres://migrator:$password@postgres-rw.postgres.svc:5432/control_plane?sslmode=verify-full&sslrootcert=/tls/ca.crt"
for job in migrate catalog-apply; do
	exists sms "$job" || create sms "$job" postgres-url "$url"
done

for declaration in contracts/grants/*.yaml; do
	service=$(basename "$declaration" .yaml)
	exists sms "$service" || create sms "$service" postgres-password "$(openssl rand -hex 32)"
done

ca=$(read_secret cert-manager internal-ca 'tls\.crt')
nonempty cert-manager/internal-ca "$ca"
printf '%s' "$ca" | kubectl -n kafka create secret generic internal-ca-cert --from-file=ca.crt=/dev/stdin --dry-run=client -o yaml | kubectl apply -f -
printf '%s' "$ca" | kubectl -n sms create configmap internal-ca --from-file=ca.crt=/dev/stdin --dry-run=client -o yaml | kubectl apply -f -
