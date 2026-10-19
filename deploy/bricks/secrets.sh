#!/usr/bin/env bash
# Crée chaque Secret absent de l'environnement de test, sans jamais remplacer un Secret présent ni afficher une
# valeur ; les valeurs passent par des descripteurs, jamais par les arguments d'un processus.
set -euo pipefail
cd "$(dirname "$0")/../.."

random() { printf '%s' "$(openssl rand -hex 32)"; }
exists() { kubectl -n "$1" get secret "$2" >/dev/null 2>&1; }
secret() { kubectl -n "$1" get secret "$2" -o "jsonpath={.data.$3}" | base64 -d; }

exists postgres migrator || kubectl -n postgres create secret generic migrator --type=kubernetes.io/basic-auth \
	--from-literal=username=migrator --from-file=password=<(random)

url() { printf 'postgres://migrator:%s@postgres-rw.postgres.svc:5432/control_plane?sslmode=verify-full&sslrootcert=/tls/ca.crt' \
	"$(secret postgres migrator password)"; }
for job in migrate catalog-apply; do
	exists sms "$job" || kubectl -n sms create secret generic "$job" --from-file=postgres-url=<(url)
done

for declaration in contracts/grants/*.yaml; do
	service=$(basename "$declaration" .yaml)
	exists sms "$service" || kubectl -n sms create secret generic "$service" --from-file=postgres-password=<(random)
done

for ca in kafka-clients-ca-cert:ca.crt:tls\\.crt:ca-cert kafka-clients-ca:ca.key:tls\\.key:ca-key; do
	IFS=: read -r name key source generation <<<"$ca"
	exists kafka "$name" && continue
	kubectl -n kafka create secret generic "$name" --from-file="$key"=<(secret cert-manager internal-ca "$source")
	kubectl -n kafka label secret "$name" strimzi.io/kind=Kafka strimzi.io/cluster=kafka
	kubectl -n kafka annotate secret "$name" "strimzi.io/$generation-generation=0"
done

kubectl -n sms create configmap internal-ca --from-file=ca.crt=<(secret cert-manager internal-ca 'tls\.crt') \
	--dry-run=client -o yaml | kubectl apply -f -
