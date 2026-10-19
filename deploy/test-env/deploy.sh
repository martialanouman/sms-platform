#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
tag=$1
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "deploy.sh: $tag n'est pas un tag de release" >&2; exit 1; }

exists() {
	for _ in $(seq 150); do kubectl -n "$1" get "$2" >/dev/null 2>&1 && return; sleep 2; done
	echo "deploy.sh: $1/$2 absent" >&2
	return 1
}
finish() {
	for _ in $(seq 600); do
		case $(kubectl -n sms get job "$1" -o jsonpath='{.status.succeeded}/{.status.conditions[?(@.type=="Failed")].status}') in
		1/*) return ;;
		*/True) echo "deploy.sh: le Job $1 a échoué" >&2; return 1 ;;
		esac
		sleep 2
	done
	return 1
}

kubectl apply -f deploy/bricks/operators.yaml
for chart in cert-manager cloudnative-pg strimzi; do
	exists kube-system "job/helm-install-$chart"
	kubectl -n kube-system wait --for=condition=complete "job/helm-install-$chart" --timeout=10m
done
kubectl -n cert-manager wait --for=condition=available deploy/cert-manager-webhook --timeout=5m

kubectl apply -f deploy/bricks/namespaces.yaml -f deploy/bricks/ca.yaml
for certificate in cert-manager/internal-ca postgres/postgres-server kafka/kafka-listener sms/catalog-apply-tls; do
	kubectl -n "${certificate%/*}" wait --for=condition=ready "certificate/${certificate#*/}" --timeout=5m
done
bash deploy/bricks/secrets.sh

kubectl -n kafka wait --for=condition=available deploy/strimzi-cluster-operator --timeout=5m
kubectl apply -f deploy/bricks/postgres.yaml -f deploy/bricks/kafka.yaml
kubectl -n postgres wait --for=condition=ready cluster/postgres --timeout=15m
kubectl -n kafka wait --for=condition=ready kafka/kafka --timeout=20m

kubectl -n sms apply -f deploy/k8s/networkpolicies.yaml
for job in migrate catalog-apply; do
	kubectl -n sms delete job "$job" --ignore-not-found --wait
	sed -E "s#^( *image: ghcr\.io/martialanouman/sms-platform/[a-z-]+)\$#\1:$tag#" "deploy/k8s/$job.yaml" | kubectl -n sms apply -f -
	finish "$job"
done
echo "deploy.sh: $tag déployé"
